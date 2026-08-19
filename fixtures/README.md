# Language Component Verification Fixtures

## Section 1. Purpose

Each fixture is a minimal project driving the commands declared by the corresponding
component under `templates/`. The `verify:*` jobs under `.gitlab/ci/` execute these
fixtures across a matrix of toolchain images and language standard values.

Every assertion inspects a produced artifact instead of a process exit code. A compiler
accepts an obsolete standard value with a warning while emitting output at a newer
version, which an exit code check fails to detect.

## Section 2. Fixture Inventory

### Item A. jvm-canary

Source syntax remains within javac 1.2 constructs. The assertion reads the class file
major version from byte offset 7, which equals the target release plus 44.

### Item B. c-canary

Source syntax remains within C89 and C++98 constructs. The `.clang-format` and
`CPPLINT.cfg` files supply the style configuration read by the `lang-c-cpp` component
when `clang_format_style` retains its default value of `file`.

### Item C. rust-canary

The `edition` key within `Cargo.toml` is rewritten by the matrix job. The committed
value is `2021`.

### Item D. csharp-canary

Source syntax remains within ISO/IEC 23270:2003 C# (ISO-1). The committed
`TargetFramework` is `net10.0`. The TFM matrix rewrites `src/Canary.csproj` and
`tests/Canary.Tests.csproj`. The language version matrix rewrites `LangVersion` on
the executable project only. The assertion reads `runtimeTarget.name` from the
produced `Canary.deps.json` and compares process stdout against `canary`.

`negative/GenericViolation.cs` uses C# 2 generics and is compiled only by
`verify:csharp-langversion-level`. `fixtures/csharp-negative` holds misformatted
input for `dotnet format --verify-no-changes`.

## Section 3. Verified Toolchain Versions

Versions recorded during verification on 2026-08-19. The matrix pins major version tags
rather than patch tags, because the assertions depend on the class file version and the
standard value acceptance, both of which remain stable across patch updates.

### Item A. JVM Images

| Image | Vendor | Observed version | Default class major |
| --- | --- | --- | --- |
| `azul/zulu-openjdk:6` | Azul Zulu | 1.6.0-119 | 50 |
| `azul/zulu-openjdk:7` | Azul Zulu | 1.7.0_352 | 51 |
| `eclipse-temurin:8-jdk` | Eclipse Adoptium | 1.8.0_492 | 52 |
| `eclipse-temurin:11-jdk` | Eclipse Adoptium | 11.0.31 | 55 |
| `eclipse-temurin:17-jdk` | Eclipse Adoptium | 17.0.19 | 61 |
| `eclipse-temurin:21-jdk` | Eclipse Adoptium | 21.0.11 LTS | 65 |
| `eclipse-temurin:25-jdk` | Eclipse Adoptium | 25.0.3 LTS | 69 |
| `maven:3.9.16-eclipse-temurin-8` | Docker Official Library | Maven 3.9.16, JDK 1.8.0_492 | 52 |
| `maven:3.9.16-eclipse-temurin-25` | Docker Official Library | Maven 3.9.16, JDK 25.0.3 | 69 |

### Item B. C, C++, and Rust Images

| Image | Vendor | Observed version |
| --- | --- | --- |
| `gcc:12` | Docker Official Library | GCC 12.5.0, GNU Make 4.3 |
| `gcc:15` | Docker Official Library | GCC 15.3.0 |
| `rust:1.97-slim` | Docker Official Library | rustc 1.97.1, cargo 1.97.1, rustup 1.29.0 |

### Item C. C# Images

MCR publishes `mcr.microsoft.com/dotnet/sdk:8.0`, `:9.0`, and `:10.0`. Observed
SDK patch versions remain unrecorded until the Draft MR pipeline executes
`verify:csharp-runtime-matrix`.

## Section 4. Verified Coverage

### Item A. JVM Target Releases

Targets 1.2 through 1.8 are reachable on `maven:3.9.16-eclipse-temurin-8`, and targets 8
through 25 are reachable on `maven:3.9.16-eclipse-temurin-25`. Two official images
therefore cover the entire reachable range.

Target 1.1 and earlier are unreachable. The javac within JDK 6, JDK 7, and JDK 8 rejects
`-source 1.1`, and no container image publishes a JDK earlier than 6. JDK 25 rejects
`--release` values below 8, which follows the retirement policy defined by JEP 182.

### Item B. C and C++ Standards

GCC 12.5.0 accepts `c89`, `c90`, `c99`, `c11`, `c17`, and `c2x`, alongside `c++98`
through `c++23`. The `c23` spelling requires GCC 14 or later, and GCC 12 reports an
unrecognized option while suggesting `c2x`.

### Item C. Rust Editions

Editions 2015, 2018, 2021, and 2024 all build and test successfully on rustc 1.97.1.

### Item D. C# Target Frameworks

The jobs encode `net8.0`, `net9.0`, and `net10.0` on `mcr.microsoft.com/dotnet/sdk:10.0`,
and a matching TFM on each of the three SDK images. Microsoft documents that an SDK
supports a fixed set of frameworks capped at the runtime it ships
([Select which .NET version to use](https://learn.microsoft.com/en-us/dotnet/core/versions/selection)).
`net11.0` on SDK 10 is the encoded floor. Outcomes await the Draft MR pipeline.

## Section 5. Toolchain Installation Paths

The `gcc:12` image ships neither `clang-format` nor `cpplint`. The `apt` and `pip`
installation fallbacks declared within `lang-c-cpp` were confirmed to resolve
`clang-format` 14.0.6 and the `cpplint` fork.

The `rust:1.97-slim` image ships `cargo`, `rust-std`, and `rustc` only, which reflects
the rustup minimal profile. The image provides `cc` and `gcc`, therefore `cargo build`
links without additional packages.

## Section 6. Excluded Images

`frekele/ant` publishes no version tag beyond `latest`, bundles an Oracle JDK 1.8.0_172
dated 2018, and omits `javap`. An Ant based fixture would require a purpose built image
derived from `eclipse-temurin:8-jdk`.

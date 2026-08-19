# Memorandum of Understanding: CI Template Toolchain Verification

Last update on Aug 19, 2026 12:00

## Section 1. Status

Empirical verification of the `lang-jvm`, `lang-c-cpp`, and `lang-rust` components. Every figure recorded in this document was produced by executing the stated commands without relying on vendor documentation.

Verification was conducted on Fedora Linux 44, kernel `7.1.8-200.fc44.x86_64`, podman 5.8.4, with SELinux in Enforcing mode. The resulting assertions are encoded as the `verify:*` jobs under `.gitlab/ci/` and driven by the test fixtures under `fixtures/`. `.gitlab-ci.yml` includes `.gitlab/ci/verify-jvm.yml`, `.gitlab/ci/verify-c-cpp.yml`, `.gitlab/ci/verify-csharp.yml`, and `.gitlab/ci/verify-rust.yml`.

`lang-csharp` now has fixtures and `verify:*` jobs. Image tags and command outcomes remain unmeasured until the Draft MR pipeline runs, as recorded in Section 10. `lang-python` and `lang-typescript` remain unverified.

## Section 2. Verification Method

### Task A. Assumptions Are Confirmed Before Adoption

Image references were confirmed through `podman manifest inspect`, which retrieves the manifest without pulling image layers. Inspection confirmed that official `openjdk` images no longer resolve, and that no container image publishes a JDK release earlier than 6, before component defaults referenced those images.

### Task B. The Two Axes Remain Separate

Coverage claims decompose into two independent axes, separated to ensure that test failures attribute to a single root cause.

| Axis    | Question                                        | Controlled by                 |
| ------- | ----------------------------------------------- | ----------------------------- |
| Runtime | Does the build tool execute on the target image | The `*_image` component input |
| Target  | Does the compiler emit for the language version | Consuming project build files |

Combining runtime and target axes produces ambiguous failures because the `-std=c23` rejection on `gcc:12` reflects GCC 12 CLI flag availability instead of C23 language incompatibility.

### Task C. Assertions Inspect the Produced Artifact

Verification assertions inspect the produced artifact. Because `javac` accepts obsolete `-source` values, emits warnings, and exits with code 0, a zero exit code fails to verify that the requested target version was honored.

| Language  | Assertion mechanism                                                               |
| --------- | --------------------------------------------------------------------------------- |
| JVM       | Class file major version read from byte offset 7, equal to target version plus 44 |
| C and C++ | Compile, execute the binary, and compare stdout against `canary`                  |
| Rust      | Build, execute the binary, compare stdout against `canary`, and assert test pass  |
| C#        | Read `runtimeTarget.name` from `Canary.deps.json`, execute, compare stdout against `canary`, and assert `dotnet test` pass |

Byte-level offset inspection replaced `javap` parsing because `javap` was absent from `frekele/ant`, and `javap` output formatting varied across JDK versions. Reading byte offset 7 depends exclusively on the standard JVM class file specification.

### Task D. Fixtures Remain Within the Oldest Dialect

Each fixture uses only constructs valid in the oldest standard under test, specifically `javac` 1.2 constructs for Java, C89 for C, C++98 for C++, and ISO/IEC 23270:2003 C# (ISO-1) for the C# executable. Failures therefore attribute directly to language standard compatibility, eliminating fixture syntax choices as a failure variable.

### Task E. Negative Tests Are Mandatory

Positive-only test suites succeed even when a linter performs no analysis. Every lint and format check is paired with an intentionally non-compliant input required to produce a non-zero exit code.

## Section 3. Image Availability

Official `openjdk` container images were withdrawn from Docker Hub, invalidating `openjdk:N` references across legacy documentation.

| Target             | Image                                                              | Result             |
| ------------------ | ------------------------------------------------------------------ | ------------------ |
| Java 1.0           | none located                                                       | No image published |
| Java 1.2           | none located                                                       | No image published |
| Java 5             | `azul/zulu-openjdk:5`, `openjdk:5`, plus four community candidates | All missing        |
| Java 6             | `azul/zulu-openjdk:6`                                              | Exists             |
| Java 6             | `openjdk:6-jdk`                                                    | Missing            |
| Java 7             | `azul/zulu-openjdk:7`                                              | Exists             |
| Java 7             | `openjdk:7-jdk`, `ibmjava:7-sdk`, `adoptopenjdk:7-jdk-hotspot`     | All missing        |
| Java 8             | `eclipse-temurin:8-jdk`                                            | Exists             |
| Java 11 through 25 | `eclipse-temurin:{11,17,21,25}-jdk`                                | All exist          |
| Maven              | `maven:3.9.16-eclipse-temurin-{8,11,17,21,25}`                     | All exist          |
| C and C++          | `gcc:12`, `gcc:15`                                                 | Both exist         |
| Rust               | `rust:1.97-slim`, `rust:1.97`                                      | Both exist         |
| C#                 | `mcr.microsoft.com/dotnet/sdk:{8.0,9.0,10.0}`                      | Tags published     |

Oracle licensing restrictions for JDK 1.x and 5.0 prohibit public redistribution, accounting for the absence of corresponding images across public container registries.

## Section 4. JVM Findings

### Task A. Runtime Axis

Each container image compiled the fixture at the native default target version of the image.

| Image                    | Vendor           | Observed version | Class major |
| ------------------------ | ---------------- | ---------------- | ----------- |
| `azul/zulu-openjdk:6`    | Azul Zulu        | 1.6.0-119        | 50          |
| `azul/zulu-openjdk:7`    | Azul Zulu        | 1.7.0_352        | 51          |
| `eclipse-temurin:8-jdk`  | Eclipse Adoptium | 1.8.0_492        | 52          |
| `eclipse-temurin:11-jdk` | Eclipse Adoptium | 11.0.31          | 55          |
| `eclipse-temurin:17-jdk` | Eclipse Adoptium | 17.0.19          | 61          |
| `eclipse-temurin:21-jdk` | Eclipse Adoptium | 21.0.11 LTS      | 65          |
| `eclipse-temurin:25-jdk` | Eclipse Adoptium | 25.0.3 LTS       | 69          |

### Task B. Target Axis on a Current JDK

`javac --release` execution on `eclipse-temurin:25-jdk`.

| Value       | Result                                   |
| ----------- | ---------------------------------------- |
| 1 through 7 | `error: release version N not supported` |
| 8           | major 52                                 |
| 9           | major 53                                 |
| 11          | major 55                                 |
| 17          | major 61                                 |
| 21          | major 65                                 |
| 25          | major 69                                 |

The minimum supported release version of 8 aligns with the retirement policy in JEP 182, under which `--release 7` was removed in JDK 20.

### Task C. Target Axis on Legacy JDKs

`javac -source X -target X` execution, recording the emitted class major version.

| Value | Zulu 6         | Zulu 7         | Temurin 8 |
| ----- | -------------- | -------------- | --------- |
| 1.1   | rejected       | rejected       | rejected  |
| 1.2   | 46             | 46             | 46        |
| 1.3   | 47             | 47             | 47        |
| 1.4   | 48             | 48             | 48        |
| 1.5   | 49             | 49             | 49        |
| 1.6   | 50             | 50             | 50        |
| 1.7   | not applicable | 51             | 51        |
| 1.8   | not applicable | not applicable | 52        |

All three compilers reject source level 1.1 with the error `javac: invalid source release: 1.1`.

### Task D. Maven End to End

Maven 3.9.16 (commit `2bdd9fddda4b155ebf8000e807eb73fd829a51d5`) execution in both container images. Expected and observed major versions matched across all twelve configurations.

| Image                             | JDK       | Property                             | Values verified   |
| --------------------------------- | --------- | ------------------------------------ | ----------------- |
| `maven:3.9.16-eclipse-temurin-8`  | 1.8.0_492 | `maven.compiler.source` and `target` | 1.2 through 1.8   |
| `maven:3.9.16-eclipse-temurin-25` | 25.0.3    | `maven.compiler.release`             | 8, 11, 17, 21, 25 |

Two official container images cover the reachable range of Java 1.2 through Java 25.

### Task E. Reachability Summary

| Requested   | Reachable | Class major    |
| ----------- | --------- | -------------- |
| Java 1.0    | No        | not applicable |
| Java 1.1    | No        | not applicable |
| Java 1.2    | Yes       | 46             |
| Java 1.4    | Yes       | 48             |
| Java SE 5.0 | Yes       | 49             |
| Java SE 6   | Yes       | 50             |
| Java SE 7   | Yes       | 51             |
| Java 8      | Yes       | 52             |
| Java 11     | Yes       | 55             |
| Java 17     | Yes       | 61             |
| Java 21     | Yes       | 65             |
| Java 25     | Yes       | 69             |

Java 1.0 and Java 1.1 are unreachable because every available `javac` compiler rejects 1.1 and no public container image publishes a JDK earlier than version 6.

### Task F. Java 1.4 Examined in Depth

Emitting class major version 48 represents the minimal guarantee implied by a support claim. The 1.4 target received detailed validation on `eclipse-temurin:8-jdk`.

| Property                                | Result                               |
| --------------------------------------- | ------------------------------------ |
| Class major at `-source/-target 1.4`    | 48                                   |
| Language level enforced                 | Yes, generics rejected               |
| `assert`, introduced in 1.4             | Compiles, major 48                   |
| Output runs on the oldest available JVM | Yes, executes on Zulu 6 at 1.6.0-119 |
| API surface restricted                  | No                                   |
| Compatible with `-Werror`               | No                                   |

Language level enforcement operates as specified. Compiling source code containing `List<String>` fails with `generics are not supported in -source 1.4`, demonstrating that the flag enforces language grammar rules in addition to bytecode versioning.

Two limitations apply to Java 1.4 verification:

1. Standard library API surface is not restricted. A class importing `java.util.concurrent.ConcurrentHashMap` (introduced in Java 5) compiled without error under `-source 1.4 -target 1.4` and produced major version 48 bytecode. Such classes fail when executed on a genuine Java 1.4 runtime, which no verified image provides.
2. The `--release` flag that restricts standard library APIs remains unavailable across legacy versions.

| Compiler | Invocation            | API restriction                                                   |
| -------- | --------------------- | ----------------------------------------------------------------- |
| JDK 8    | `--release`           | Flag does not exist, reports `invalid flag: --release`            |
| JDK 25   | `--release 8`         | Restricts, rejects the Java 9 `List.of` with `cannot find symbol` |
| JDK 25   | `-source 8 -target 8` | No restriction, compiles `List.of` and emits major 52             |

No available container image enforces API surface restrictions for target versions below Java 8. Restricting the API surface requires the `-bootclasspath` parameter pointing to a genuine Java 1.4 runtime library, which is unavailable in published container images. This limitation applies to targets 1.2, 1.3, 1.5, 1.6, and 1.7.

The `-Werror` flag is incompatible with source level 1.4. Compiling at source level 1.4 emits four compiler warnings regarding the unset bootstrap class path alongside obsolete source and target options. Promoting warnings to errors fails the build on compiler options independently of source code defects. Builds combining these settings require `-Xlint:-options`.

## Section 5. C and C++ Findings

### Task A. C Standards

| Value | `gcc:12` at 12.5.0 | `gcc:15` at 15.3.0 |
| ----- | ------------------ | ------------------ |
| c89   | pass               | pass               |
| c90   | pass               | not retested       |
| c99   | pass               | pass               |
| c11   | pass               | pass               |
| c17   | pass               | pass               |
| c2x   | pass               | pass               |
| c23   | rejected           | pass               |

GCC 12 rejects `-std=c23` with `unrecognized command-line option '-std=c23'; did you mean '-std=c2x'?`. The `-std=c23` option was introduced in GCC 14. Verification compiled and executed each test binary, comparing stdout against `canary`.

### Task B. C++ Standards

On `gcc:12`, language levels `c++98`, `c++03`, `c++11`, `c++14`, `c++17`, `c++20`, and `c++23` all compiled and executed successfully. C++98 support requires no specialized container image. The encoded `verify:cpp-std-matrix` job on `gcc:15` lists `c++98`, `c++11`, `c++17`, `c++20`, and `c++23`.

### Task C. Component Command Paths

| Check                                              | Result                                                     |
| -------------------------------------------------- | ---------------------------------------------------------- |
| `clang-format` present in `gcc:12`                 | Absent, `apt` fallback resolved Debian clang-format 14.0.6 |
| `cpplint` present in `gcc:12`                      | Absent, `pip` fallback resolved the cpplint fork           |
| `clang-format --style=file` honors `.clang-format` | Misformatted input reformatted to four-space indent        |
| `cpplint` upon the clean fixture                   | Exit 0                                                     |
| `make all`                                         | Both binaries built and executed                           |

### Task D. Encoded Jobs

The Section 5 measurements are encoded in `.gitlab/ci/verify-c-cpp.yml`. Shared `rules` live on the hidden job `.c-cpp-verify`. The four visible jobs extend that hidden job.

| Job                     | Image and matrix                                                                                                                 | Assertion                                                                                                      |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `verify:c-std-matrix`   | `gcc:12` with `C_STANDARDS` `c89 c90 c99 c11 c17 c2x`; `gcc:15` with `c89 c99 c11 c17 c2x c23`                                    | `gcc -std=$std` on `src/canary.c`, stdout equals `canary`                                                      |
| `verify:cpp-std-matrix` | `gcc:12` with `CPP_STANDARDS` `c++98` through `c++23`; `gcc:15` with `c++98 c++11 c++17 c++20 c++23`                              | `g++ -std=$std` on `src/canary.cpp`, stdout equals `canary`                                                    |
| `verify:c-cpp-make`     | `gcc:12`                                                                                                                         | `make all`, stdout of `./bin/canary` and `./bin/canary_cpp` equals `canary`                                    |
| `verify:c-cpp-tooling`  | `gcc:12`                                                                                                                         | `cpplint` and `clang-format --dry-run --Werror` on the canary sources, plus the Section 7 negative cases       |

## Section 6. Rust Findings

### Task A. Image Assumptions

The `rust:1.97-slim` image provides rustc 1.97.1 (commit `8bab26f4f`, 2026-07-14), cargo 1.97.1, and rustup 1.29.0. Installed toolchain components comprise `cargo`, `rust-std`, and `rustc`, confirming the rustup minimal profile alongside the absence of rustfmt and clippy. Both `cc` and `gcc` are present, allowing binary linking without additional system packages.

### Task B. Job Commands

`rustup component add rustfmt` and `rustup component add clippy` execute successfully. The validation sequence comprising `cargo fmt --all`, `cargo clippy --all-targets -- -D warnings`, `cargo build --all-targets`, and `cargo test` completes with the `tests::canary_runs` test case passing.

### Task C. Editions

Rust editions 2015, 2018, 2021, and 2024 build and execute successfully on rustc 1.97.1.

## Section 7. Negative Test Findings

| Case                | Input                                             | Expected | Observed                  |
| ------------------- | ------------------------------------------------- | -------- | ------------------------- |
| cpplint             | Tab indent, 84 column line, comment without space | Non-zero | Exit 1, three findings    |
| clang-format        | Misformatted braces and operators                 | Non-zero | Exit 1                    |
| clippy              | `clone` upon an `i32`, triggering `clone_on_copy` | Non-zero | Exit 101                  |
| Maven release floor | `maven.compiler.release=7` on JDK 25              | Non-zero | Exit 1, no class emitted  |
| Java source level   | `List<String>` under `-source 1.4`                | Non-zero | Exit 1, generics rejected |

Configuration inheritance from parent directories was confirmed. Fixtures under `negative/` inherit `.clang-format` and `CPPLINT.cfg` from the fixture root, and `clang-format --dump-config` confirmed `IndentWidth: 4`.

### Task A. Stale Artifact Finding

The Maven release floor test case initially returned exit code 0 due to an omitted `rm -rf target` command, which allowed the compiler plugin to skip compilation when an existing class file was present.

Repeated verification established the defect mechanism: seeding `target/` with major version 69 bytecode followed by executing `maven.compiler.release=7` returned exit code 0 while leaving the class file at major version 69. Stale build artifacts conceal compilation failures and invalid target versions. The matrix test loop therefore deletes the output directory on every iteration.

## Section 8. Excluded Images

`frekele/ant` was evaluated and rejected. While an Ant build targeting Java 1.2 succeeded and produced major version 46 bytecode, four properties disqualify the image from a pinned catalog:

| Property    | Observed                                          |
| ----------- | ------------------------------------------------- |
| Tags        | `latest` only, `1.10.5` and `1.9.14` both missing |
| Bundled JDK | Oracle JDK 1.8.0_172                              |
| Image date  | 2018-06-14, 592 MB                                |
| `javap`     | Absent                                            |

Ant testing requires a custom container image derived from `eclipse-temurin:8-jdk` with Apache Ant installed from official archives at a pinned release version.

## Section 9. Incidents During Verification

| Incident                               | Cause                                                                                | Resolution                                              |
| -------------------------------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------- |
| `javap` parsing produced no output     | `javap` absent from `frekele/ant`                                                    | Byte offset 7 inspection                                |
| SELinux relabel refused                | `:Z` upon the repository root attempted to relabel `selinux/gitlab_runner_podman.pp` | Mount each fixture directory individually               |
| Maven plugin resolution failed         | `-o` offline flag with an empty local repository                                     | Removed the flag, shared a local repository across runs |
| Negative case returned exit 0          | Omitted `rm -rf target`                                                              | Corrected, and retained as the Section 7 finding        |
| `javac` reported `directory not found` | `-d out` requires the directory to exist beforehand                                  | Created the directory within the same invocation        |

## Section 10. Coverage Not Yet Established

| Component               | State                                                                      |
| ----------------------- | -------------------------------------------------------------------------- |
| `lang-csharp`           | Jobs and fixtures encoded. Tag existence confirmed on MCR. Command outcomes, SDK patch versions, and LangVersion acceptance await the Draft MR pipeline |
| `lang-python`           | Not executed. The `pip` and `uv` branches both remain unverified           |
| `lang-typescript`       | Not executed. A fixture requires the monorepo layout the component assumes |
| `lang-jvm` fmt and lint | Both inputs default to empty, therefore neither command path is exercised  |

Three additional boundaries define the verification scope. Coverage verifies compiler target output only, without executing test suites on actual Java 6 or Java 7 runtime environments. Standard library API fidelity below Java 8 remains unenforced due to container image limitations, as documented in Section 4. Task F. The `verify:cpp-std-matrix` job on `gcc:15` lists five C++ language versions. The same job on `gcc:12` lists the seven versions recorded in Section 5. Task B.

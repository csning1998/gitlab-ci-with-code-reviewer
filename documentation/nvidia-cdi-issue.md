# NVIDIA GPU CDI Issue

## Section 1. Automatic CDI Specification Regeneration on Driver Update

### Item A. Technical Rationale and Failure Modes

1. **Failure Modes**:
    - Driver updates DO NOT trigger automatic Container Device Interface (CDI) specification regeneration under Fedora `golang-github-nvidia-container-toolkit` due to missing systemd auto-regeneration units.
    - Package manager transactions under `dnf5` SHALL NOT load legacy `dnf4` plugins (`python3-dnf-plugin-post-transaction-actions`).
    - Package manager transaction hooks SHALL NOT capture driver updates initiated by `akmods` compilation or manual driver reinstallations.

2. **Monitoring Requirement**:
    - Systemd path monitoring MUST target `/usr/lib64/libnvidia-ml.so.1` to ensure change detection across all installation vectors (`dnf5`, `akmods`, manual reinstallation).

3. **Configuration Inheritance Rules**:
    - Per [`containers.conf.5.md`](https://github.com/containers/common/blob/main/docs/containers.conf.5.md),

        > _The default behavior during the loading sequence of multiple containers.conf files is to override previous data._

        Evaluation of multiple configuration files SHALL replace the `cdi_spec_dirs` array sequentially unless an entry explicitly specifies the `{append=true}` attribute.

    - Restricting `cdi_spec_dirs` to `~/.config/cdi` SHALL override default system paths, causing engine isolation from `/etc/cdi/nvidia.yaml`. System-wide automated regeneration SHALL operate on `/etc/cdi/nvidia.yaml` without requiring per-user path overrides.

### Item B. Implementation Procedure

Deployment MAY follow Option B.1 (System-Wide Path) or Option B.2 (Dual-Path Target). Option B.1 is RECOMMENDED to eliminate configuration redundancy.

#### Option B.1 (RECOMMENDED): System-Wide Path Specification

1. Purge per-user configuration overrides if present:

    ```zsh
    rm -f ~/.config/containers/containers.conf
    ```

    If `~/.config/containers/containers.conf` contains non-CDI `[engine]` definitions, remove only the `cdi_spec_dirs` key assignment.

2. Install system-wide CDI generation script:

    ```zsh
    sudo tee /usr/local/bin/nvidia-cdi-refresh.sh > /dev/null <<'EOF'
    #!/bin/bash
    set -euo pipefail
    nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
    EOF
    sudo chmod 755 /usr/local/bin/nvidia-cdi-refresh.sh
    ```

3. Configure systemd monitoring and execution units as specified in Option B.2 Steps 2 and 3.

#### Option B.2: Dual-Path Specification Regeneration

1. Install dual-path CDI generation script:

    ```zsh
    sudo tee /usr/local/bin/nvidia-cdi-refresh.sh > /dev/null <<'EOF'
    #!/bin/bash
    set -euo pipefail
    nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
    for spec in /home/*/.config/cdi/nvidia.yaml; do
        [ -f "$spec" ] || continue
        user=$(stat -c '%U' "$spec")
        runuser -u "$user" -- nvidia-ctk cdi generate --output="$spec"
    done
    EOF
    sudo chmod 755 /usr/local/bin/nvidia-cdi-refresh.sh
    ```

2. Create systemd path unit `/etc/systemd/system/nvidia-cdi-refresh.path` and service unit `/etc/systemd/system/nvidia-cdi-refresh.service`:

    ```zsh
    sudo tee /etc/systemd/system/nvidia-cdi-refresh.path > /dev/null <<'EOF'
    [Unit]
    Description=Watch NVIDIA driver library for version changes

    [Path]
    PathChanged=/usr/lib64/libnvidia-ml.so.1

    [Install]
    WantedBy=multi-user.target
    EOF

    sudo tee /etc/systemd/system/nvidia-cdi-refresh.service > /dev/null <<'EOF'
    [Unit]
    Description=Regenerate NVIDIA CDI specs after a driver change

    [Service]
    Type=oneshot
    ExecStart=/usr/local/bin/nvidia-cdi-refresh.sh
    EOF
    ```

3. Enable and activate path monitoring unit:

    ```zsh
    sudo systemctl daemon-reload
    sudo systemctl enable --now nvidia-cdi-refresh.path
    ```

### Item C. Operational Constraints

1. **Target Invariance**: Path monitoring target MUST be `/usr/lib64/libnvidia-ml.so.1` to maintain continuous change detection regardless of shared library point-version changes.

2. **Single-Path Scope (Option B.1)**:
    - The refresh execution script MUST update `/etc/cdi/nvidia.yaml` exclusively.
    - The refresh execution script SHALL NOT read or modify user-level CDI specifications.

3. **Dual-Path Scope (Option B.2)**:
    - The refresh execution script MUST update `/etc/cdi/nvidia.yaml` and pre-existing user specifications matching `/home/*/.config/cdi/nvidia.yaml`.
    - The refresh execution script SHALL NOT instantiate new user-level CDI specifications for user accounts lacking pre-existing `~/.config/cdi/nvidia.yaml` files.

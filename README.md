# FINS CLI

## Installation

To install FINS CLI, run the provided installation script:

```bash
curl -fsSL https://raw.githubusercontent.com/Han-Yu-Meng/fins-cli/main/install.sh | sudo bash
```

### Build Internal Tools

For the first time, you need to build the necessary tools for Agent and Inspect functionality:

```bash
fins agent build
fins inspect build
```

### Help

View all available commands:

```bash
fins --help
```

## Uninstallation

To completely remove FINS CLI from your system:

```bash
curl -fsSL https://raw.githubusercontent.com/Han-Yu-Meng/fins-cli/main/uninstall.sh | sudo bash
```
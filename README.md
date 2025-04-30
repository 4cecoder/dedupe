# Screenshot Deduplicator

A command-line utility to organize and deduplicate screenshot files efficiently.

## Features

- Finds and organizes screenshots across your system (specifically within your home directory or a specified source).
- Detects duplicate files using **concurrent MD5 hash comparison** for improved speed.
- Intelligently **skips irrelevant directories** (like `.git`, `node_modules`, `venv`, `Library`, `Applications`, `Movies`, etc.) during scans.
- **Filters files by size** (skips hashing files < 5KB or > 25MB) to avoid processing non-screenshots and speed up scans.
- **Automatically finds destination:** If the default (`~/Pictures/Screenshots`) or specified destination doesn't exist, it searches common locations (`~/Documents`, `~/Pictures`, `~/Desktop`) for a `Screenshots` (or similar) folder.
- Correctly handles cases where the **destination is inside the source** directory.
- Renames files that have the same name but different content.
- Provides a safe **dry-run mode** to preview actions before execution.
- Displays a **dynamic progress bar** during scanning, showing files scanned, found, hashed, elapsed time, and current path.
- Supports various screenshot naming patterns from different operating systems.
- Can work with any file type using the `--all` flag, not just screenshots.
- Cross-platform compatibility (Windows, macOS, Linux).

## Installation

### Prerequisites

- Go 1.18 or higher

### Dependencies

The project uses Go modules to manage dependencies. The main dependencies are:

- [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) - For terminal styling and formatting.
- [github.com/vbauerster/mpb/v8](https://github.com/vbauerster/mpb/v8) - For the dynamic progress bars.

Go will automatically download dependencies when you build the project.

### Building from source

#### Option 1: Using Go commands

```bash
# Clone the repository
git clone https://github.com/4cecoder/dedupe.git
cd dedupe

# Build the executable
go build -o dedupe

# Optional: Install to your system (Unix/Linux/macOS)
sudo cp dedupe /usr/local/bin/
```

#### Option 2: Using the Makefile

```bash
# Clone the repository
git clone https://github.com/4cecoder/dedupe.git
cd dedupe

# Build the binary
make build

# Install to your system (Unix/Linux/macOS)
sudo make install
```

The Makefile provides the following targets:

| Target        | Description                                                 |
| :------------ | :---------------------------------------------------------- |
| `make build`  | Builds the `dedupe` binary                                  |
| `make install`| Builds and installs the binary to `/usr/local/bin` (macOS/Linux) |
| `make clean`  | Removes build artifacts                                     |
| `make help`   | Displays help information about available targets             |

#### Windows-specific instructions

On Windows, you can:

1. Build using Go directly:
   ```bash
   go build -o dedupe.exe
   ```

2. Or use the Makefile with Git Bash or WSL:
   ```bash
   make build
   ```

3. After building, manually copy the executable `dedupe.exe` to a directory in your PATH, or run it directly from its location.

## Usage

```bash
dedupe [OPTIONS]
```

### Options

| Flag            | Alias | Description                                                                                           | Default                    |
| :-------------- | :---- | :---------------------------------------------------------------------------------------------------- | :------------------------- |
| `--help`        | `-h`  | Show help message and exit                                                                            |                            |
| `--execute`     | `-e`  | Actually execute the operations (copy/delete/rename).                                                 | `false` (Dry Run)          |
| `--delete`      | `-d`  | Delete duplicate source files (only applies in execute mode).                                         | `false`                    |
| `--verbose`     | `-v`  | Show detailed information (warnings, errors) during scanning below the progress bar.                  | `true`                     |
| `--source DIR`  | `-s`  | Specify source directory. Ignored if `--scan-home` is used.                                         | `~/Desktop`                |
| `--destination DIR` | `-dst`| Specify destination directory.                                                                        | `~/Pictures/Screenshots` * |
| `--all`         | `-a`  | Process all files, not just screenshots (respects size limits for hashing).                           | `false`                    |
| `--scan-home`   |       | Scan entire home directory (`~/`) for source files, intelligently skipping common system/dev folders. | `false`                    |

* *Note: If the default or specified destination doesn't exist, the tool will automatically search for a `Screenshots` (or similar) folder in `~/Documents`, `~/Pictures`, and `~/Desktop`.* 

### Examples

```bash
# Run in dry-run mode (default) using Desktop as source and default destination
dedupe

# Scan entire home directory for screenshots and move/delete duplicates
dedupe --scan-home --execute --delete

# Use custom source and destination directories, process all file types
dedupe --source ~/Downloads --destination ~/Organized/Files --all --execute

# Use verbose mode to see potential errors during scan
dedupe --scan-home -v
```

## How It Works

1.  **Determine Source:** Identifies the source directory (default `~/Desktop`, specified with `-s`, or `~/` with `--scan-home`).
2.  **Determine Destination:** Identifies the destination (default `~/Pictures/Screenshots`, specified with `-dst`, or found via fallback search in `~/Documents`, `~/Pictures`, `~/Desktop`).
3.  **Scan Source:** Scans the source directory concurrently with a progress bar.
    - Skips common irrelevant directories (`.git`, `node_modules`, `venv`, `Library`, etc.) and hidden folders.
    - Excludes the destination subdirectory if it's nested within the source.
    - Queues files matching name patterns (or all files with `--all`) within size limits (5KB-25MB) for hashing.
4.  **Scan Destination:** Scans the destination directory separately.
5.  **Concurrent Hashing:** Worker goroutines calculate MD5 hashes for queued files.
6.  **Analyze & Classify:** Compares source files (from step 3) against destination files (from step 4):
    - **Duplicates:** Source file hash matches a destination file hash.
    - **Name Collisions:** Source file hash is unique, but its name exists in the destination.
    - **Unique Files:** Source file name and hash are both unique relative to the destination.
7.  **Execute (or Dry Run):**
    - **Dry Run (Default):** Reports planned actions (copy, delete, rename) without changing anything.
    - **Execute (`-e`):** Copies unique files and renamed files to the destination.
    - **Execute + Delete (`-e -d`):** Deletes duplicate source files instead of copying unique/renamed ones.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Workflow

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Coding Standards

- Follow standard Go formatting and idioms (`gofmt`).
- Add comments for public functions and complex logic.
- Update documentation (`README.md`) when adding or changing features.

## Acknowledgments

- Built with [lipgloss](https://github.com/charmbracelet/lipgloss) for terminal styling.
- Uses [mpb](https://github.com/vbauerster/mpb/v8) for progress bars.
- From [Bytecats.codes](https://bytecats.codes) by [4cecoder](https://github.com/4cecoder).

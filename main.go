package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Terminal styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	headingStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginTop(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#2D88FF"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00AA00"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA00"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))

	uniqueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00AA00")).
			Bold(true)

	duplicateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF7700")).
			Bold(true)

	existsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#2D88FF")).
			Bold(true)

	dryRunStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Italic(true)

	highlightStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF00FF"))

	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#999999"))

	attributionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666")).
				Italic(true)
)

// FileInfo contains information about a file
type FileInfo struct {
	Path string
	Hash string
	Size int64
	Name string
}

// Define a structure to pass jobs to workers
type hashJob struct {
	path string
	info os.FileInfo
}

// Define a structure for results from workers
type hashResult struct {
	fileInfo FileInfo
	err      error
}

func main() {
	// Default configuration
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error finding home directory: %v", err)))
		return
	}

	sourceDir := filepath.Join(homeDir, "Desktop")
	destDir := filepath.Join(homeDir, "Pictures", "Screenshots")
	dryRun := true
	deleteMode := false
	verboseMode := true
	screenshotOnly := true
	scanHome := false

	// Parse command-line arguments
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--execute" || arg == "-e" {
			dryRun = false
		} else if arg == "--delete" || arg == "-d" {
			deleteMode = true
		} else if arg == "--verbose" || arg == "-v" {
			verboseMode = true
		} else if arg == "--help" || arg == "-h" {
			printHelp()
			return
		} else if arg == "--source" || arg == "-s" {
			if i+1 < len(os.Args) {
				sourceDir = os.Args[i+1]
				i++
			}
		} else if arg == "--destination" || arg == "-dst" {
			if i+1 < len(os.Args) {
				destDir = os.Args[i+1]
				i++
			}
		} else if arg == "--all" || arg == "-a" {
			screenshotOnly = false
		} else if arg == "--scan-home" {
			scanHome = true
		}
	}

	// Validate source directory exists OR scanHome is enabled
	if !scanHome {
		sourceInfo, err := os.Stat(sourceDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println(errorStyle.Render(fmt.Sprintf("Source directory doesn't exist: %s (Use --scan-home to search everywhere)", sourceDir)))
				return
			}
			fmt.Println(errorStyle.Render(fmt.Sprintf("Error accessing source directory: %v", err)))
			return
		}

		if !sourceInfo.IsDir() {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Source path is not a directory: %s", sourceDir)))
			return
		}
	} else {
		// If scanHome is true, sourceDir becomes the home directory
		sourceDir = homeDir
		fmt.Println(infoStyle.Render("Scan Mode:"), warningStyle.Render("Scanning entire home directory (~/)"))
	}

	fmt.Println(titleStyle.Render(" Dedupe - Screenshot Organizer & Deduplicator "))
	fmt.Println(infoStyle.Render("Source:"), pathStyle.Render(sourceDir))

	// Resolve tilde and check destination directory
	if strings.HasPrefix(destDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Error getting home directory: %v", err)))
			return
		}
		destDir = filepath.Join(home, destDir[2:])
	}

	_, err = os.Stat(destDir)
	if err != nil && os.IsNotExist(err) {
		// Destination doesn't exist, try fallbacks
		fmt.Println(warningStyle.Render(fmt.Sprintf("Destination directory %s not found. Searching fallback locations recursively (depth 5)...", destDir)))
		foundFallback := false
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Error getting home directory for fallback search: %v", homeErr)))
			return
		}

		fallbackBases := []string{
			filepath.Join(home, "Documents"),
			filepath.Join(home, "Pictures"),
			filepath.Join(home, "Desktop"),
		}

		maxDepth := 5

		for _, base := range fallbackBases {
			if _, statErr := os.Stat(base); os.IsNotExist(statErr) {
				continue // Skip base directory if it doesn't exist
			}

			baseDepth := len(strings.Split(filepath.Clean(base), string(os.PathSeparator)))

			walkErr := filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					// Ignore permission errors silently during fallback search,
					// print other warnings if verbose.
					if os.IsPermission(walkErr) {
						return nil // Skip this entry silently
					}
					if verboseMode {
						// Cannot easily hide bar here as it's in a different scope
						fmt.Printf("\r%s\n", warningStyle.Render(fmt.Sprintf("  Warning during fallback search in %s: %v", path, walkErr)))
					}
					return nil // Skip entry but continue walk
				}

				// Calculate current depth relative to the base
				currentDepth := len(strings.Split(filepath.Clean(path), string(os.PathSeparator))) - baseDepth

				if d.IsDir() {
					if currentDepth >= maxDepth {
						// Stop searching deeper in this branch
						return filepath.SkipDir
					}
					// Check if this directory is named one of the target variations
					targetNames := []string{"Screenshots", "screenshot", "Screenshot", "screenshots"}
					for _, targetName := range targetNames {
						if strings.EqualFold(d.Name(), targetName) {
							fmt.Println(infoStyle.Render("Found alternative destination:"), pathStyle.Render(path))
							destDir = path
							foundFallback = true
							return filepath.SkipAll // Found it, stop entire walk
						}
					}
				}
				return nil // Continue walking
			})

			if walkErr != nil && walkErr != filepath.SkipAll {
				fmt.Println(errorStyle.Render(fmt.Sprintf("Error during fallback search in %s: %v", base, walkErr)))
				// Continue checking other base directories
			}

			if foundFallback {
				break // Stop checking other base directories if found
			}
		}

		if !foundFallback {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Could not find a directory named 'Screenshots' (or variations) within 5 levels of %s, %s, or %s.",
				filepath.Join(home, "Documents"), filepath.Join(home, "Pictures"), filepath.Join(home, "Desktop"))))
			fmt.Println(infoStyle.Render("Please specify a valid destination directory using -dst or --destination, or create one."))
			return // Exit if no destination found
		}
	} else if err != nil {
		// Other error accessing destination directory
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error accessing destination directory %s: %v", destDir, err)))
		return
	}

	fmt.Println(infoStyle.Render("Destination:"), pathStyle.Render(destDir))

	// Prevent running if destination is inside source
	// cleanedSource, _ := filepath.Abs(sourceDir)
	// cleanedDest, _ := filepath.Abs(destDir)
	// if strings.HasPrefix(cleanedDest+string(os.PathSeparator), cleanedSource+string(os.PathSeparator)) && cleanedDest != cleanedSource {
	// 	fmt.Println(errorStyle.Render("Error: Destination directory cannot be inside the source directory."))
	// 	fmt.Println(infoStyle.Render(fmt.Sprintf("  Source:      %s", cleanedSource)))
	// 	fmt.Println(infoStyle.Render(fmt.Sprintf("  Destination: %s", cleanedDest)))
	// 	fmt.Println(infoStyle.Render("Please choose a destination outside the source path."))
	// 	return
	// }

	if screenshotOnly {
		fmt.Println(infoStyle.Render("Mode:"), "Screenshot files only")
	} else {
		fmt.Println(infoStyle.Render("Mode:"), "All files")
	}

	if dryRun {
		fmt.Println(warningStyle.Render("⚠ DRY RUN MODE: No changes will be made"))
	}
	if deleteMode {
		fmt.Println(warningStyle.Render("⚠ DELETE MODE: Duplicate files will be deleted"))
	}

	// Process files
	processFiles(sourceDir, destDir, dryRun, deleteMode, verboseMode, screenshotOnly)
}

func printHelp() {
	fmt.Println(titleStyle.Render(" Dedupe - Screenshot Organizer & Deduplicator "))
	fmt.Println(headingStyle.Render("Usage:"), "dedupe [OPTIONS]")
	fmt.Println("\nFinds, organizes, and deduplicates screenshots.")
	fmt.Println("\nOptions:")
	fmt.Println("  -h, --help                Show this help message and exit")
	fmt.Println("  -e, --execute             Actually execute the operations (default is dry run mode)")
	fmt.Println("  -d, --delete              Delete duplicate files (only applies in execute mode)")
	fmt.Println("  -v, --verbose             Show detailed information about all files")
	fmt.Println("  -s, --source DIR          Specify source directory (default: ~/Desktop)")
	fmt.Println("  -dst, --destination DIR   Specify destination directory (default: ~/Pictures/Screenshots)")
	fmt.Println("  -a, --all                 Process all files, not just screenshots")
	fmt.Println("  --scan-home             Scan entire home directory for screenshots (ignores --source)")
	fmt.Println("\nIn dry run mode, the script will show what would be done without making any changes.")
}

func processFiles(sourceDir, destDir string, dryRun, deleteMode, verboseMode, screenshotOnly bool) {
	// Find all files in source directory
	scanMsg := "\n🔍 Scanning for screenshots..."
	if !screenshotOnly {
		scanMsg = "\n🔍 Scanning for all files..."
	}
	fmt.Println(headingStyle.Render(scanMsg))

	// Determine if destination is inside source
	absSourceDir, _ := filepath.Abs(sourceDir)
	absDestDir, _ := filepath.Abs(destDir)
	isDestInsideSource := strings.HasPrefix(absDestDir+string(os.PathSeparator), absSourceDir+string(os.PathSeparator)) && absDestDir != absSourceDir

	destToSkip := ""
	if isDestInsideSource {
		destToSkip = absDestDir
		fmt.Println(infoStyle.Render(fmt.Sprintf("Scanning source directory (%s), excluding destination subdirectory (%s)", pathStyle.Render(sourceDir), pathStyle.Render(destDir))))
	} else {
		fmt.Println(infoStyle.Render(fmt.Sprintf("Scanning source directory (%s)", pathStyle.Render(sourceDir))))
	}

	sourceFiles, err := findFiles(sourceDir, screenshotOnly, destToSkip, verboseMode) // Pass verboseMode
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error finding source files: %v", err)))
		return
	}
	fmt.Println(infoStyle.Render(fmt.Sprintf("Found %d potential source files", len(sourceFiles))))

	// Ensure destination path is absolute for reliable comparison
	// absDestDir, err := filepath.Abs(destDir) // Already calculated above
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error getting absolute path for destination %s: %v", destDir, err)))
		return
	}

	// Filter out source files that are already inside the destination directory
	// filteredSourceFiles := []FileInfo{}
	// for _, sf := range sourceFiles {
	// 	absSourceFileDir, _ := filepath.Abs(filepath.Dir(sf.Path))
	// 	// Add separator to avoid matching partial directory names
	// 	if !strings.HasPrefix(absSourceFileDir+string(os.PathSeparator), absDestDir+string(os.PathSeparator)) {
	// 		filteredSourceFiles = append(filteredSourceFiles, sf)
	// 	}
	// }

	// if len(filteredSourceFiles) != len(sourceFiles) {
	// 	fmt.Println(infoStyle.Render(fmt.Sprintf("Filtered %d source files located within the destination directory", len(sourceFiles)-len(filteredSourceFiles))))
	// }
	// sourceFiles = filteredSourceFiles // Use the filtered list from now on

	// Find all files in destination directory
	fmt.Println(infoStyle.Render(fmt.Sprintf("Scanning destination directory (%s)", pathStyle.Render(destDir))))
	destFiles, err := findFiles(destDir, screenshotOnly, "", verboseMode) // Pass verboseMode
	if err != nil {
		// If the destination wasn't found/accessible initially, this might happen.
		// The main func already checks this, but maybe handle it more gracefully here?
		// For now, just printing the error as before.
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error finding destination files (%s): %v", pathStyle.Render(destDir), err)))
		return
	}
	fmt.Println(infoStyle.Render(fmt.Sprintf("Found %d existing files in destination directory", len(destFiles))))

	// Analyze files
	fmt.Println(headingStyle.Render("\n📊 Analyzing files..."))
	uniqueCount, duplicateCount := analyzeAndProcessFiles(sourceFiles, destFiles, sourceDir, destDir, dryRun, deleteMode, verboseMode)

	// Print summary
	fmt.Println(headingStyle.Render("\n📋 Summary:"))
	fmt.Printf("- Total potential source files found (excluding destination subdir if applicable): %d\n", len(sourceFiles))
	fmt.Printf("- %s: %d\n", uniqueStyle.Render("Unique files"), uniqueCount)
	fmt.Printf("- %s: %d\n", duplicateStyle.Render("Duplicate files"), duplicateCount)

	if dryRun {
		fmt.Println(warningStyle.Render("\n✓ Completed DRY RUN. No files were actually moved, copied, or deleted."))
		fmt.Println(dryRunStyle.Render("Run with -e or --execute flag to perform the actual operations."))
	} else {
		fmt.Println(successStyle.Render("\n✓ Organization completed successfully."))
	}

	// Add attribution
	attributionText := "Built by https://github.com/4cecoder from https://bytecats.codes"
	fmt.Println(attributionStyle.Render("\n" + attributionText))
}

func findFiles(dir string, screenshotOnly bool, dirToSkip string, verbose bool) ([]FileInfo, error) {
	var files []FileInfo
	var scannedCount atomic.Int64
	var foundCount atomic.Int64  // Files matching criteria sent for hashing
	var hashedCount atomic.Int64 // Files successfully hashed
	var currentPath atomic.Value // Stores the path currently being scanned
	homeDir, _ := os.UserHomeDir()

	// Directories to skip by absolute path (usually in home)
	skipAbsDirs := map[string]bool{
		filepath.Join(homeDir, "Library"):                                  true, // macOS system Library
		filepath.Join(homeDir, "Pictures", "Photo Booth Library"):          true, // macOS Photo Booth
		filepath.Join(homeDir, "Pictures", "Photos Library.photoslibrary"): true, // macOS Photos
		filepath.Join(homeDir, "Applications"):                             true,
		filepath.Join(homeDir, "Public"):                                   true,
		filepath.Join(homeDir, "Movies"):                                   true,
		filepath.Join(homeDir, "Music"):                                    true,
		filepath.Join(homeDir, "VirtualBox VMs"):                           true,
		filepath.Join(homeDir, "Parallels"):                                true,
		filepath.Join(homeDir, "anaconda3"):                                true,
		filepath.Join(homeDir, "miniconda3"):                               true,
		filepath.Join(homeDir, "go"):                                       true, // Go workspace/cache
	}

	// Directory names to skip regardless of location
	skipNameDirs := map[string]bool{
		".git":         true,
		".cache":       true,
		"node_modules": true,
		"__pycache__":  true,
		"build":        true,
		"dist":         true,
		"vendor":       true,
		"venv":         true,
		"venvs":        true, // Add venvs (e.g., from pipx)
		"deps":         true, // Add deps
	}

	// --- Initialize Progress Bar (mpb) ---
	p := mpb.New(mpb.WithWidth(80))

	bar := p.AddBar(0, // Total estimated later
		// Custom style attempt removed due to compiler errors
		// mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding("-").Rbound("]"),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Scanning %s ", pathStyle.Render(filepath.Base(dir)))),
		),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth), // Hashed / Found
			decor.Name(" | Files: "),
			decor.Any(func(s decor.Statistics) string { return fmt.Sprintf("%d", scannedCount.Load()) }, decor.WCSyncSpace),
			decor.Name(" | "),
			decor.Elapsed(decor.ET_STYLE_GO, decor.WCSyncWidth),
			decor.Name(" | Path: "),
			decor.Any(func(s decor.Statistics) string {
				pathVal := currentPath.Load()
				if pathVal == nil {
					return "..."
				}
				pathStr := pathVal.(string)
				maxLen := 25
				if len(pathStr) > maxLen {
					return "..." + pathStr[len(pathStr)-maxLen:]
				}
				return pathStr
			}, decor.WCSyncSpace),
		),
	)

	// --- Concurrency Setup ---
	numWorkers := runtime.NumCPU()
	jobs := make(chan hashJob, numWorkers*2)
	results := make(chan hashResult, numWorkers*2)
	var wg sync.WaitGroup

	// Start workers
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				hash, err := calculateMD5(job.path)
				if err != nil {
					results <- hashResult{err: fmt.Errorf("worker %d hash error for %s: %w", workerID, job.path, err)}
					continue
				}
				results <- hashResult{
					fileInfo: FileInfo{
						Path: job.path,
						Hash: hash,
						Size: job.info.Size(),
						Name: job.info.Name(),
					},
				}
				hashedCount.Add(1)
				bar.Increment() // Increment bar when hash is done
			}
		}(w)
	}

	// --- Goroutine to collect results ---
	var collectionErr error
	collectionDone := make(chan struct{})
	go func() {
		for result := range results {
			if result.err != nil {
				if verbose {
					// mpb handles terminal redrawing better, direct print might be ok
					// but using SetTotal to hide is safer
					bar.SetTotal(0, true) // Hide bar
					fmt.Printf("\r%s\n", warningStyle.Render(result.err.Error()))
				}
				if collectionErr == nil {
					collectionErr = result.err
				}
				continue
			}
			files = append(files, result.fileInfo)
		}
		close(collectionDone)
	}()

	// --- Directory Walk ---
	var walkErr error
	currentPath.Store(".")
	walkErr = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		scannedCount.Add(1)
		// Don't increment bar here, only when hashing

		relPath, _ := filepath.Rel(dir, path)
		currentPath.Store(relPath)

		if err != nil {
			if verbose {
				bar.SetTotal(0, true) // Hide bar
				fmt.Printf("\r%s\n", warningStyle.Render(fmt.Sprintf("  Warning accessing %s: %v", path, err)))
			}
			return nil
		}

		if d.IsDir() {
			// Check if this is the directory we need to skip (e.g., destination within source)
			if dirToSkip != "" {
				absPath, _ := filepath.Abs(path)
				if absPath == dirToSkip {
					if verbose {
						// No longer print skipping message
					}
					return filepath.SkipDir
				}
			}

			// Skip specific absolute paths
			absPath, _ := filepath.Abs(path) // Get abs path once
			if skipAbsDirs[absPath] {
				return filepath.SkipDir
			}

			// Skip specific directory names
			if skipNameDirs[d.Name()] {
				if verbose {
					// No longer print skipping message
				}
				return filepath.SkipDir
			}
			// Skip hidden directories (except .local/share which might have Flatpak screenshots)
			if strings.HasPrefix(d.Name(), ".") && d.Name() != ".local" && !strings.HasPrefix(path, filepath.Join(homeDir, ".local", "share")) {
				if verbose {
					// No longer print skipping message
				}
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			if verbose {
				bar.SetTotal(0, true) // Hide bar
				fmt.Printf("\r%s\n", warningStyle.Render(fmt.Sprintf("Warning: Could not get info for %s: %v", path, infoErr)))
			}
			return nil
		}

		if screenshotOnly && !isScreenshot(path) {
			return nil
		}

		const minSizeForHash = 5 * 1024
		const maxSizeForHash = 25 * 1024 * 1024
		if info.Size() < minSizeForHash || info.Size() > maxSizeForHash {
			return nil
		}

		// Update total estimate for the bar before sending job
		newFoundTotal := foundCount.Add(1)
		bar.SetTotal(newFoundTotal, false)

		jobs <- hashJob{path: path, info: info}

		return nil
	})

	// Mark the bar as completed now that the walk is done
	// and all jobs have been sent. Bar increments as results come in.
	bar.SetTotal(foundCount.Load(), true)

	// --- Cleanup ---
	close(jobs)
	wg.Wait()
	close(results)
	<-collectionDone
	p.Wait() // Wait for mpb container

	if walkErr != nil {
		return files, walkErr
	}
	return files, collectionErr
}

func isScreenshot(path string) bool {
	// Check for common screenshot naming patterns across different OS platforms
	name := filepath.Base(path)
	lowerName := strings.ToLower(name)

	// Common screenshot filename patterns
	patterns := []string{
		"screen shot", "screenshot", "screen capture", "screencapture",
		"screen", "capture", "snip", "snippingtool", "snipping",
		"scrnshot", "printscreen", "print_screen", "screengrab",
	}

	// Check against patterns
	for _, pattern := range patterns {
		if strings.Contains(lowerName, pattern) {
			// Check if it has common screenshot extensions
			return strings.HasSuffix(lowerName, ".png") ||
				strings.HasSuffix(lowerName, ".jpg") ||
				strings.HasSuffix(lowerName, ".jpeg") ||
				strings.HasSuffix(lowerName, ".bmp")
		}
	}

	// Check for timestamp-like filenames with screenshot extensions
	// Like "2023-04-30_12-34-56.png"
	isTimestampPattern := regexp.MustCompile(`^\d{4}[-_]\d{2}[-_]\d{2}[-_T]?\d{2}[-_:]\d{2}[-_:]\d{2}.*\.(png|jpg|jpeg|bmp)$`).MatchString(lowerName)

	return isTimestampPattern
}

func calculateMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func analyzeAndProcessFiles(sourceFiles, destFiles []FileInfo, sourceDir, destDir string, dryRun, deleteMode, verboseMode bool) (int, int) {
	// Create a map of hashes to destination files
	destHashMap := make(map[string]FileInfo)
	for _, file := range destFiles {
		destHashMap[file.Hash] = file
	}

	// Create a map of names to destination files
	destNameMap := make(map[string]FileInfo)
	for _, file := range destFiles {
		destNameMap[file.Name] = file
	}

	uniqueCount := 0
	duplicateCount := 0

	// Process each source file
	for _, file := range sourceFiles {
		// Skip files that are already in the destination directory
		if strings.HasPrefix(file.Path, destDir) {
			continue
		}

		// Check if file with same content already exists in destination
		if destFile, exists := destHashMap[file.Hash]; exists {
			// Found a duplicate (identical content)
			duplicateCount++

			if verboseMode {
				fmt.Printf("%s %s %s %s\n",
					duplicateStyle.Render("[DUPLICATE]"),
					highlightStyle.Render(file.Name),
					infoStyle.Render("(identical to)"),
					destFile.Name)
			}

			// Delete the duplicate if delete mode is enabled and not dry run
			if deleteMode && !dryRun {
				fmt.Printf("%s %s\n",
					warningStyle.Render("Deleting duplicate:"),
					pathStyle.Render(file.Path))
				if err := os.Remove(file.Path); err != nil {
					fmt.Println(errorStyle.Render(fmt.Sprintf("Error deleting file: %v", err)))
				}
			} else if deleteMode && dryRun {
				fmt.Printf("%s %s\n",
					dryRunStyle.Render("[PRETEND] Would delete duplicate:"),
					pathStyle.Render(file.Path))
			}
		} else {
			// This is a unique file (not in destination based on hash)
			// Only process/copy uniques if NOT in delete mode
			if !deleteMode {
				uniqueCount++

				if verboseMode {
					fmt.Printf("%s %s\n",
						uniqueStyle.Render("[UNIQUE] "), // Added space for alignment
						highlightStyle.Render(file.Name))
				}

				// Check if a file with the same name already exists
				if _, exists := destNameMap[file.Name]; exists {
					// Same name but different content
					newName := renameFile(file.Name)
					if verboseMode {
						fmt.Printf("  %s\n  %s %s\n",
							infoStyle.Render("File with same name but different content exists."),
							infoStyle.Render("Will rename to:"),
							highlightStyle.Render(newName))
					}

					// Copy file to destination with new name
					if !dryRun {
						destPath := filepath.Join(destDir, newName)
						fmt.Printf("%s %s\n",
							successStyle.Render("Copying renamed to:"),
							pathStyle.Render(destPath))
						if err := copyFile(file.Path, destPath); err != nil {
							fmt.Println(errorStyle.Render(fmt.Sprintf("Error copying file: %v", err)))
						}
					} else {
						destPath := filepath.Join(destDir, newName)
						fmt.Printf("%s %s → %s\n",
							dryRunStyle.Render("[PRETEND] Would copy renamed:"),
							pathStyle.Render(file.Path),
							pathStyle.Render(destPath))
					}
				} else {
					// Unique name and content
					if !dryRun {
						destPath := filepath.Join(destDir, file.Name)
						fmt.Printf("%s %s\n",
							successStyle.Render("Copying unique to:"),
							pathStyle.Render(destPath))
						if err := copyFile(file.Path, destPath); err != nil {
							fmt.Println(errorStyle.Render(fmt.Sprintf("Error copying file: %v", err)))
						}
					} else {
						destPath := filepath.Join(destDir, file.Name)
						fmt.Printf("%s %s → %s\n",
							dryRunStyle.Render("[PRETEND] Would copy unique:"),
							pathStyle.Render(file.Path),
							pathStyle.Render(destPath))
					}
				}
			}
		}
	}

	return uniqueCount, duplicateCount
}

// Rename a file by adding a timestamp
func renameFile(name string) string {
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	timestamp := fmt.Sprintf("_%d", time.Now().Unix())
	return base + timestamp + ext
}

// Copy a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

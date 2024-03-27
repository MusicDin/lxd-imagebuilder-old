package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

type buildOptions struct {
	global *globalOptions

	StreamVersion string
	ImageDirs     []string
	Workers       int
}

func (o *buildOptions) NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build <path> [flags]",
		Short:   "Build simplestream index on the given path",
		GroupID: "main",
		RunE:    o.Run,
	}

	cmd.PersistentFlags().StringVar(&o.StreamVersion, "stream-version", "v1", "Stream version")
	cmd.PersistentFlags().StringSliceVarP(&o.ImageDirs, "image-dir", "d", []string{"images"}, "Image directory (relative to path argument)")
	cmd.PersistentFlags().IntVar(&o.Workers, "workers", max(runtime.NumCPU()/2, 1), "Maximum number of concurrent operations")

	return cmd
}

func (o *buildOptions) Run(_ *cobra.Command, args []string) error {
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("Argument %q is required and cannot be empty", "path")
	}

	return buildIndex(o.global.ctx, args[0], o.StreamVersion, o.ImageDirs, o.Workers)
}

// replace struct holds old and new path for a file replace.
type replace struct {
	OldPath string
	NewPath string
}

func buildIndex(ctx context.Context, rootDir string, streamVersion string, streamNames []string, workers int) error {
	metaDir := path.Join(rootDir, "streams", streamVersion)

	var replaces []replace
	index := stream.NewStreamIndex()

	// Ensure meta directory exists.
	err := os.MkdirAll(metaDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Create metadata directory: %w", err)
	}

	// Create product catalogs by reading image directories.
	for _, streamName := range streamNames {
		// Create product catalog from directory structure.
		catalog, err := buildProductCatalog(ctx, rootDir, streamVersion, streamName, workers)
		if err != nil {
			return err
		}

		// Write product catalog to a temporary file that is located next
		// to the final file to ensure atomic replace. Temporary file is
		// prefixed with a dot to hide it.
		catalogPath := filepath.Join(metaDir, fmt.Sprintf("%s.json", streamName))
		catalogPathTemp := filepath.Join(metaDir, fmt.Sprintf(".%s.json.tmp", streamName))

		err = shared.WriteJSONFile(catalogPathTemp, catalog)
		if err != nil {
			return err
		}

		defer os.Remove(catalogPathTemp)

		replaces = append(replaces, replace{
			OldPath: catalogPathTemp,
			NewPath: catalogPath,
		})

		// Relative path for index.
		catalogRelPath, err := filepath.Rel(rootDir, catalogPath)
		if err != nil {
			return err
		}

		// Add index entry.
		index.AddEntry(streamName, catalogRelPath, *catalog)
	}

	// Write index to a temporary file that is located next to the
	// final file to ensure atomic replace. Temporary file is
	// prefixed with a dot to hide it.
	indexPath := filepath.Join(metaDir, "index.json")
	indexPathTemp := filepath.Join(metaDir, ".index.json.tmp")

	err = shared.WriteJSONFile(indexPathTemp, index)
	if err != nil {
		return err
	}

	defer os.Remove(indexPathTemp)

	// Index file should be updated last, once all catalog files
	// are in place.
	replaces = append(replaces, replace{
		OldPath: indexPathTemp,
		NewPath: indexPath,
	})

	// Move temporary files to final destinations.
	for _, r := range replaces {
		err := os.Rename(r.OldPath, r.NewPath)
		if err != nil {
			return err
		}

		// Set read permissions.
		err = os.Chmod(r.NewPath, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// buildProductCatalog compares the existing product catalog and actual products on
// the disk. For missing products, first the delta files and hashes are calculated
// and only then the products are inserted into the catalog. Workers are used to
// limit maximum concurent tasks when calulcating hashes and delta files.
func buildProductCatalog(ctx context.Context, rootDir string, streamVersion string, streamName string, workers int) (*stream.ProductCatalog, error) {
	// Get current product catalog (from json file).
	catalogPath := filepath.Join(rootDir, "streams", streamVersion, fmt.Sprintf("%s.json", streamName))
	catalog, err := shared.ReadJSONFile(catalogPath, &stream.ProductCatalog{})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if catalog == nil {
		catalog = stream.NewCatalog(nil)
	}

	// Get existing products (from actual directory hierarchy).
	products, err := stream.GetProducts(rootDir, streamName)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	var mutex sync.Mutex // To safely update the catalog.Products map

	// Ensure at least 1 worker is spawned.
	if workers < 1 {
		workers = 1
	}

	// Job queue.
	jobs := make(chan func(), workers)
	defer close(jobs)

	// Create new pool of workers.
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}

					job()
				}
			}
		}()
	}

	// Extract new (unreferenced products and product versions) and add them
	// to the catalog.
	_, newProducts := diffProducts(catalog.Products, products)
	for id, p := range newProducts {
		if len(p.Versions) == 0 {
			continue
		}

		productPath := filepath.Join(streamName, p.RelPath())

		_, ok := catalog.Products[id]
		if !ok {
			// If product does not exist yet, set the product value to one
			// that is fetched from the directory hierarchy. This ensures
			// that the product id and other metadata is set. However,
			// remove existing versions, as they will be repopulated below.
			product := products[id]
			product.Versions = make(map[string]stream.Version, len(p.Versions))

			// Lock before updating, as another gorotine may be accessing it.
			mutex.Lock()
			catalog.Products[id] = product
			mutex.Unlock()
		}

		for versionName := range p.Versions {
			// Add a job for processing a new version.
			wg.Add(1)
			jobs <- func() {
				defer wg.Done()

				// Create delta files before retrieving the version,
				// so that hashes are also calculated for delta files.
				err = createDeltaFiles(ctx, rootDir, productPath, versionName)
				if err != nil {
					slog.Error("Failed to create delta file", "streamName", streamName, "product", id, "version", versionName, "error", err)
					return
				}

				// Read the version and generate the file hashes.
				versionPath := filepath.Join(productPath, versionName)
				version, err := stream.GetVersion(rootDir, versionPath, true)
				if err != nil {
					slog.Error("Failed to get version", "streamName", streamName, "product", id, "version", versionName, "error", err)
					return
				}

				// Verify items checksums if checksum file is present
				// within the version. If verification succeeds, update
				// the checksums file to include potential delta files.
				if version.Checksums != nil {
					checksumFile := filepath.Join(rootDir, versionPath, stream.FileChecksumSHA256)

					for _, item := range version.Items {
						checksum, ok := version.Checksums[item.Name]

						// If checksums for delta files do not exist,
						// append them to the checksums file because
						// they were just generated.
						if !ok && (item.Ftype == stream.ItemTypeDiskKVMDelta || item.Ftype == stream.ItemTypeSquashfsDelta) {
							err := shared.AppendToFile(checksumFile, fmt.Sprintf("%s  %s\n", checksum, item.Name))
							if err != nil {
								slog.Error("Failed to update checksums file", "streamName", streamName, "product", id, "version", versionName, "error", err)
								return
							}

							continue
						}

						// Verify checksum.
						if checksum != item.SHA256 {
							slog.Error("Checksum mismatch", "streamName", streamName, "product", id, "version", versionName, "item", item.Name)
							return
						}
					}
				}

				mutex.Lock()
				catalog.Products[id].Versions[versionName] = *version
				mutex.Unlock()

				slog.Info("New version added to the product catalog", "streamName", streamName, "product", id, "version", versionName)
			}
		}
	}

	// Wait for all goroutines to finish.
	wg.Wait()

	return catalog, nil
}

// createDeltaFiles traverses through the directory of the given stream and
// creates missing delta (.vcdiff) files for any subsequent complete versions.
func createDeltaFiles(ctx context.Context, rootDir string, productRelPath string, versionName string) error {
	productPath := filepath.Join(rootDir, productRelPath)

	// Get existing products (from actual directory hierarchy).
	product, err := stream.GetProduct(rootDir, productRelPath)
	if err != nil {
		return err
	}

	versions := shared.MapKeys(product.Versions)
	slices.Sort(versions)

	if len(versions) < 2 {
		// At least 2 versions must be available for diff.
		return nil
	}

	// Skip the oldest version because even if the .vcdiff does
	// not exist, we cannot generate it.
	for i := 1; i < len(versions); i++ {
		if versionName != "" && versions[i] != versionName {
			continue
		}

		preName := versions[i-1]
		curName := versions[i]

		version := product.Versions[curName]

		for _, item := range version.Items {
			// Vcdiff should be created only for qcow2 and squashfs files.
			if item.Ftype != stream.ItemTypeDiskKVM && item.Ftype != stream.ItemTypeSquashfs {
				continue
			}

			prefix, _ := strings.CutSuffix(item.Name, filepath.Ext(item.Name))
			suffix := "vcdiff"

			if item.Ftype == stream.ItemTypeDiskKVM {
				suffix = "qcow2.vcdiff"
			}

			vcdiff := fmt.Sprintf("%s.%s.%s", prefix, preName, suffix)
			_, ok := version.Items[vcdiff]
			if ok {
				// Delta already exists. Skip..
				slog.Debug("Delta already exists", "version", curName, "deltaBase", preName)
				continue
			}

			sourcePath := filepath.Join(productPath, preName, item.Name)
			targetPath := filepath.Join(productPath, curName, item.Name)
			outputPath := filepath.Join(productPath, curName, vcdiff)

			// Ensure source path exists.
			_, err := os.Stat(sourcePath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					// Source does not exist. Skip..
					continue
				}

				return err
			}

			err = calcVCDiff(ctx, sourcePath, targetPath, outputPath)
			if err != nil {
				return err
			}

			slog.Info("Delta generated successfully", "version", curName, "deltaBase", preName)
		}
	}

	return nil
}

// calcVCDiff calculates the delta file (.vcdiff) between the source and target
// files. The output file is written to the outputPath.
func calcVCDiff(ctx context.Context, sourcePath string, targetPath string, outputPath string) error {
	bin, err := exec.LookPath("xdelta3")
	if err != nil {
		return err
	}

	// -e compress
	// -f force
	cmd := exec.CommandContext(ctx, bin, "-e", "-s", sourcePath, targetPath, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		_ = os.Remove(outputPath)
		return err
	}

	return nil
}

// DiffProducts is a helper function that compares two product maps and returns
// the difference between them.
func diffProducts(oldProducts map[string]stream.Product, newProducts map[string]stream.Product) (old map[string]stream.Product, new map[string]stream.Product) {
	old = make(map[string]stream.Product) // Extra (old) products.
	new = make(map[string]stream.Product) // Missing (new) products.

	// Extract new products and versions.
	for id, p := range newProducts {
		_, ok := oldProducts[id]
		if !ok {
			// Product is missing in the old catalog.
			new[id] = p
			continue
		}

		for name, v := range p.Versions {
			_, ok := oldProducts[id].Versions[name]
			if !ok {
				// Version is missing in the old catalog.
				new[id].Versions[name] = v
			}
		}
	}

	// Extract old products and versions.
	for id, p := range oldProducts {
		_, ok := newProducts[id]
		if !ok {
			// Product is missing in the new catalog.
			old[id] = p
			continue
		}

		for name, v := range p.Versions {
			_, ok := newProducts[id].Versions[name]
			if !ok {
				// Version is missing in the new catalog.
				old[id].Versions[name] = v
			}
		}
	}

	return old, new
}

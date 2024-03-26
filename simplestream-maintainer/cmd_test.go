package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/testutils"
)

func TestBuildIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name          string
		Mock          testutils.ProductMock
		WantErrString string
		WantCatalog   stream.ProductCatalog
		WantIndex     stream.StreamIndex
	}{
		{
			Name: "Ensure empty index and catalog are created",
			Mock: testutils.MockProduct("images/ubuntu/lunar/amd64/cloud"),
			WantCatalog: stream.ProductCatalog{
				ContentID: "images",
				Format:    "products:1.0",
				DataType:  "image-downloads",
				Products:  map[string]stream.Product{},
			},
			WantIndex: stream.StreamIndex{
				Format: "index:1.0",
				Index: map[string]stream.StreamIndexEntry{
					"images": {
						Path:     "streams/v1/images.json",
						Format:   "products:1.0",
						Datatype: "image-downloads",
						Updated:  time.Now().Format(time.RFC3339),
						Products: []string{},
					},
				},
			},
		},
		{
			// Ensures:
			// - Incomplete versions are ignored in the catalog.
			// - Delta is calculated for the previous complete version.
			// - Missing source file for calculating delta does not break index building.
			Name: "Ensure incomplete versions are ignored, and vcdiffs are calculated only for complete versions",
			Mock: testutils.MockProduct("images-daily/ubuntu/focal/amd64/cloud").AddVersions(
				testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz", "disk.qcow2"), // Missing rootfs.squashfs
				testutils.MockVersion("2024_01_02").WithFiles("lxd.tar.xz"),               // Incomplete version
				testutils.MockVersion("2024_01_03").WithFiles("lxd.tar.xz", "disk.qcow2", "rootfs.squashfs"),
			),
			WantCatalog: stream.ProductCatalog{
				ContentID: "images",
				Format:    "products:1.0",
				DataType:  "image-downloads",
				Products: map[string]stream.Product{
					"ubuntu:focal:amd64:cloud": {
						Aliases:      "ubuntu/focal/cloud",
						Architecture: "amd64",
						Distro:       "ubuntu",
						Release:      "focal",
						Variant:      "cloud",
						Requirements: map[string]string{},
						Versions: map[string]stream.Version{
							"2024_01_01": {
								Checksums: map[string]string{},
								Items: map[string]stream.Item{
									"lxd.tar.xz": {
										Ftype:                    "lxd.tar.xz",
										Size:                     12,
										Path:                     "images-daily/ubuntu/focal/amd64/cloud/2024_01_01/lxd.tar.xz",
										SHA256:                   "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
										CombinedSHA256DiskKvmImg: "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
									},
									"disk.qcow2": {
										Ftype:  "disk-kvm.img",
										Size:   12,
										Path:   "images-daily/ubuntu/focal/amd64/cloud/2024_01_01/disk.qcow2",
										SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
									},
								},
							},
							"2024_01_03": {
								Checksums: map[string]string{},
								Items: map[string]stream.Item{
									"lxd.tar.xz": {
										Ftype:                    "lxd.tar.xz",
										Size:                     12,
										Path:                     "images-daily/ubuntu/focal/amd64/cloud/2024_01_03/lxd.tar.xz",
										SHA256:                   "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
										CombinedSHA256DiskKvmImg: "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
										CombinedSHA256SquashFs:   "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
									},
									"disk.qcow2": {
										Ftype:  "disk-kvm.img",
										Size:   12,
										Path:   "images-daily/ubuntu/focal/amd64/cloud/2024_01_03/disk.qcow2",
										SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
									},
									// Ensure vcdiff is calculated for disk.qcow2 with delta base 2024_01_01.
									"disk.2024_01_01.qcow2.vcdiff": {
										Ftype:     "disk-kvm.img.vcdiff",
										Size:      45,
										Path:      "images-daily/ubuntu/focal/amd64/cloud/2024_01_03/disk.2024_01_01.qcow2.vcdiff",
										SHA256:    "db7efd312bacbb1a8ca8d52f4da37052081ac86f63f93f8f62b52ae455079db2",
										DeltaBase: "2024_01_01",
									},
									"rootfs.squashfs": {
										Ftype:  "squashfs",
										Size:   12,
										Path:   "images-daily/ubuntu/focal/amd64/cloud/2024_01_03/rootfs.squashfs",
										SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
									},
								},
							},
						},
					},
				},
			},
			WantIndex: stream.StreamIndex{
				Format: "index:1.0",
				Index: map[string]stream.StreamIndexEntry{
					"images-daily": {
						Path:     "streams/v1/images-daily.json",
						Format:   "products:1.0",
						Datatype: "image-downloads",
						Updated:  time.Now().Format(time.RFC3339),
						Products: []string{
							"ubuntu:focal:amd64:cloud",
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock
			p.Create(t, t.TempDir())

			err := buildIndex(context.Background(), p.RootDir(), "v1", []string{p.StreamName()}, 2)
			require.NoError(t, err, "Failed building index and catalog files!")

			// Convert expected catalog and index files to json.
			jsonCatalogExpect, err := json.MarshalIndent(test.WantCatalog, "", "  ")
			require.NoError(t, err)

			jsonIndexExpect, err := json.MarshalIndent(test.WantIndex, "", "  ")
			require.NoError(t, err)

			// Read actual catalog and index files.
			catalogPath := filepath.Join(p.RootDir(), "streams", "v1", fmt.Sprintf("%s.json", p.StreamName()))
			indexPath := filepath.Join(p.RootDir(), "streams", "v1", "index.json")

			jsonCatalogActual, err := os.ReadFile(catalogPath)
			require.NoError(t, err)

			jsonIndexActual, err := os.ReadFile(indexPath)
			require.NoError(t, err)

			// Ensure index and catalog json files match.
			require.Equal(t,
				strings.TrimSpace(string(jsonCatalogExpect)),
				strings.TrimSpace(string(jsonCatalogActual)),
				"Expected catalog does not match the built one!")

			require.Equal(t,
				strings.TrimSpace(string(jsonIndexExpect)),
				strings.TrimSpace(string(jsonIndexActual)),
				"Expected index does not match the built one!")
		})
	}
}

func TestBuildProductCatalog_ChecksumVerification(t *testing.T) {
	t.Parallel()

	checksums := []string{
		fmt.Sprintf("%s  lxd.tar.xz", testutils.ItemDefaultContentSHA), // Valid
		fmt.Sprintf("%s  disk.qcow2", testutils.ItemDefaultContentSHA), // Valid
		fmt.Sprintf("%s  r.squashfs", testutils.ItemDefaultContentSHA), // Valid
		"invalid-sha256-checksum  invalid.squashfs",                    // Invalid
		"invalid-sha256-checksum  invalid.qcow2",                       // Invalid
	}

	tests := []struct {
		Name         string
		Mock         testutils.ProductMock
		WantVersions []string // List of expected versions in the final product catalog.
	}{
		{
			Name: "Ensure checksum validation is ignored when checksum file is missing",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").AddVersions(
				testutils.MockVersion("v1").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
				testutils.MockVersion("v2").WithFiles("lxd.tar.xz", "root.squashfs"),
				testutils.MockVersion("v3").WithFiles("lxd.tar.xz", "disk.qcow2")),
			WantVersions: []string{
				"v1",
				"v2",
				"v3",
			},
		},
		{
			Name: "Ensure versions with mismatched checksums are excluded from the product catalog",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").AddVersions(
				testutils.MockVersion("v1").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "invalid.qcow2"),
				testutils.MockVersion("v2").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "invalid.squashfs")),
			WantVersions: []string{},
		},
		{
			Name: "Ensure version is excluded if checksum file exists, but checksum for a certain item is missing",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").AddVersions(
				testutils.MockVersion("v1").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "no-sha.qcow2"),
				testutils.MockVersion("v2").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "no-sha.squashfs")),
			WantVersions: []string{},
		},
		{
			Name: "Ensure version with mismatched checksums is excluded but product catalog is still created",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").AddVersions(
				testutils.MockVersion("v1").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "r.squashfs"),
				testutils.MockVersion("v2").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "r.squashfs", "invalid.qcow2"),
				testutils.MockVersion("v3").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "disk.qcow2")),
			WantVersions: []string{
				"v1",
				"v3",
			},
		},
		{
			Name: "Ensure only valid versions are included in the product catalog.",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").AddVersions(
				testutils.MockVersion("v1").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "disk.qcow2", "r.squashfs"), // Valid: All checksums match
				testutils.MockVersion("v2").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "missing.squashfs"),         // Invalid: Missing checksum
				testutils.MockVersion("v3").SetChecksums(checksums...).WithFiles("lxd.tar.xz", "invalid.qcow2")),           // Invalid: Invalid checksum
			WantVersions: []string{
				"v1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock
			p.Create(t, t.TempDir())

			// Build product catalog.
			catalog, err := buildProductCatalog(context.Background(), p.RootDir(), "v1", p.StreamName(), 2)
			require.NoError(t, err, "Failed building index and catalog files!")

			// Fetch the product from catalog by its id.
			productID := strings.Join(strings.Split(p.RelPath(), "/")[1:], ":")
			product, ok := catalog.Products[productID]

			// Ensure product and all expected product versions are found.
			require.True(t, ok, "Product not found in the catalog!")
			require.ElementsMatch(t, test.WantVersions, shared.MapKeys(product.Versions))
		})
	}
}

func TestPruneOldVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name          string
		Mock          testutils.ProductMock
		KeepVersions  int
		WantErrString string
		WantVersions  []string
	}{
		{
			Name:          "Validation | Retain number too low",
			KeepVersions:  0,
			WantErrString: "At least 1 product version must be retained",
		},
		{
			Name: "Ensure no error on empty product catalog",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddProductCatalog(),
			KeepVersions: 1,
			WantVersions: []string{},
		},
		{
			Name: "Ensure exact number of versions is kept",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(
					testutils.MockVersion("01").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
					testutils.MockVersion("02").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
					testutils.MockVersion("03").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2")).
				AddProductCatalog(),
			KeepVersions: 3,
			WantVersions: []string{
				"01",
				"02",
				"03",
			},
		},
		{
			Name: "Ensure the given number of product versions is retained",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(
					testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
					testutils.MockVersion("2024_01_05").WithFiles("lxd.tar.xz", "root.squashfs"),
					testutils.MockVersion("2024_05_01").WithFiles("lxd.tar.xz", "disk.squashfs"),
					testutils.MockVersion("2025_01_01").WithFiles("lxd.tar.xz", "disk.qcow2")).
				AddProductCatalog(),
			KeepVersions: 3,
			WantVersions: []string{
				"2024_01_05",
				"2024_05_01",
				"2025_01_01",
			},
		},
		{
			Name: "Ensure only complete versions are retained",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(
					testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz"),                                // Incomplete
					testutils.MockVersion("2024_01_02").WithFiles("lxd.tar.xz", "root.squashfs"),               // Complete
					testutils.MockVersion("2024_01_03").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"), // Complete
					testutils.MockVersion("2024_01_04").WithFiles("root.squashfs"),                             // Incomplete
					testutils.MockVersion("2024_01_05").WithFiles("lxd.tar.xz", "disk.qcow2"),                  // Complete
					testutils.MockVersion("2024_01_06").WithFiles("disk.qcow2")).                               // Incomplete
				AddProductCatalog(),
			KeepVersions: 2,
			WantVersions: []string{
				"2024_01_03",
				"2024_01_05",
			},
		},
		{
			Name: "Ensure only referenced versions are prunned",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(
					testutils.MockVersion("2023").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2024").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2025").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2026").WithFiles("lxd.tar.xz", "disk.qcow2")).
				AddProductCatalog().
				AddVersions(
					testutils.MockVersion("2027").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2028").WithFiles("lxd.tar.xz", "disk.qcow2")),
			KeepVersions: 2,
			WantVersions: []string{
				"2025",
				"2026",
				"2027",
				"2028",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock
			p.Create(t, t.TempDir())

			err := pruneStreamProductVersions(p.RootDir(), "v1", p.StreamName(), test.KeepVersions)
			if test.WantErrString == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, test.WantErrString)
				return
			}

			product, err := stream.GetProduct(p.RootDir(), p.RelPath())
			require.NoError(t, err)

			// Ensure expected product versions are found.
			require.ElementsMatch(t, test.WantVersions, shared.MapKeys(product.Versions))
		})
	}
}

func TestPruneDanglingResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name         string
		Mock         testutils.ProductMock
		WantProducts map[string][]string // product: list of versions
	}{
		{
			Name: "Ensure no error on empty product catalog",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddProductCatalog(),
			WantProducts: map[string][]string{},
		},
		{
			Name: "Ensure referenced product version is not removed",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(testutils.MockVersion("1.0").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2")).
				AddProductCatalog(),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"1.0",
				},
			},
		},
		{
			Name: "Ensure referenced product version older then 1 day is not removed",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(testutils.MockVersion("1.0").WithFiles("lxd.tar.xz", "disk.qcow2")).
				AddProductCatalog().
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"1.0",
				},
			},
		},
		{
			Name: "Ensure fresh unreferenced product version is not removed",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(testutils.MockVersion("1.0").WithFiles("lxd.tar.xz", "disk.qcow2")).
				AddProductCatalog().
				AddVersions(testutils.MockVersion("2.0").WithFiles("lxd.tar.xz", "root.squashfs")),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"1.0",
					"2.0",
				},
			},
		},
		{
			Name: "Ensure unreferenced old product is removed",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(testutils.MockVersion("1.0").WithFiles("lxd.tar.xz", "disk.qcow2")).
				AddProductCatalog().
				AddVersions(testutils.MockVersion("2.0").WithFiles("lxd.tar.xz", "root.squashfs")).
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"1.0",
				},
			},
		},
		{
			Name: "Ensure unreferenced old product is not removed when product catalog is not empty",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddProductCatalog().
				AddVersions(testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz", "root.squashfs")).
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
				},
			},
		},
		{
			Name: "Ensure only unreferenced project versions are removed",
			Mock: testutils.MockProduct("images/ubuntu/noble/amd64/cloud").
				AddVersions(
					testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2024_01_02").WithFiles("lxd.tar.xz", "root.squashfs")).
				AddProductCatalog().
				AddVersions(
					testutils.MockVersion("2024_01_03").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("2024_01_04").WithFiles("lxd.tar.xz", "root.squashfs")).
				SetFilesAge(48 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
					"2024_01_02",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock
			p.Create(t, t.TempDir())

			err := pruneDanglingProductVersions(p.RootDir(), "v1", p.StreamName())
			require.NoError(t, err)

			products, err := stream.GetProducts(p.RootDir(), p.StreamName())
			require.NoError(t, err)

			// Ensure all expected products are found.
			require.Equal(t, shared.MapKeys(test.WantProducts), shared.MapKeys(products))

			// Ensure all expected product versions are found.
			for pid, p := range products {
				require.ElementsMatch(t, test.WantProducts[pid], shared.MapKeys(p.Versions))
			}
		})
	}
}

func TestPruneEmptyDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		TestName     string
		Structure    []string // Created before test. Suffix "/" represents directory.
		ExpectRemove []string
		ExpectExists []string
	}{
		{
			TestName:     "Test non-empty dir",
			Structure:    []string{"root/file"},
			ExpectExists: []string{"root"},
		},
		{
			TestName:     "Test empty dir",
			Structure:    []string{"root/"},
			ExpectRemove: []string{"root"},
		},
		{
			TestName:     "Test nested empty dirs",
			Structure:    []string{"root/parent/child/empty/"},
			ExpectRemove: []string{"root"},
		},
		{
			TestName: "Test partial parent removal",
			Structure: []string{
				"root/parent/empty/",
				"root/file",
			},
			ExpectRemove: []string{"root/parent"},
			ExpectExists: []string{"root"},
		},
		{
			TestName: "Test multiple dirs",
			Structure: []string{
				"root/parent-1/child-1/empty/",
				"root/parent-1/child-2/",
				"root/parent-2/child-1/empty/",
				"root/parent-2/child-2/",
				"root/parent-2/file",
				"root/parent-3/child-1/non-empty/file",
			},
			ExpectRemove: []string{
				"root/parent-1",
				"root/parent-2/child-1",
				"root/parent-2/child-2",
				"root/parent-2/child-3",
			},
			ExpectExists: []string{
				"root/parent-2",
				"root/parent-3/child-1/non-empty/",
			},
		},
		{
			TestName: "Test unclean dirs",
			Structure: []string{
				"root1/./file",
				"root2/../root2/file",
				"root3/../root3/gone/",
				"/root4/file",
				"//root5//.//..//root5//empty///file",
			},
			ExpectExists: []string{
				"root1",
				"root2",
				"root4",
				"root5/empty",
			},
			ExpectRemove: []string{
				"root3/gone",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.TestName, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Ensure all expected directories and files exist.
			for _, f := range test.Structure {
				path := filepath.Join(tmpDir, f)

				if strings.HasSuffix(f, "/") {
					err := os.MkdirAll(path, os.ModePerm)
					require.NoError(t, err, "Failed creating temporary directory")
				} else {
					err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
					require.NoError(t, err, "Failed creating temporary directory")

					err = os.WriteFile(path, []byte{}, os.ModePerm)
					require.NoError(t, err, "Failed creating temporary file")
				}
			}

			err := pruneEmptyDirs(tmpDir, true)
			require.NoError(t, err)

			// Check expected remaining directories.
			for _, f := range test.ExpectExists {
				path := filepath.Join(tmpDir, f)
				require.DirExists(t, path, "Directory (or file) was unexpectedly pruned!")
			}

			// Check expected removed directories.
			for _, f := range test.ExpectRemove {
				path := filepath.Join(tmpDir, f)
				require.NoDirExists(t, path, "Directory was expected to be pruned, but still exists!")
			}
		})
	}
}

func TestPruneEmptyDirs_KeepRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name     string
		FullPath string
		RootPath string
	}{
		{
			Name:     "Test empty dir itself",
			FullPath: "root",
			RootPath: "root",
		},
		{
			Name:     "Test nested empty dirs",
			FullPath: "root/child",
			RootPath: "root",
		},
		{
			Name:     "Test unclean: Double slashed",
			FullPath: "root///child//",
			RootPath: "//root///",
		},
		{
			Name:     "Test unclean: Self-reference",
			FullPath: "root/././child/./.",
			RootPath: "./root/./././",
		},
		{
			Name:     "Test unclean: Redundant change dir",
			FullPath: "root/child/../../root/child/../child",
			RootPath: "root/../root",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Merge using Sprintf to preserve unclean paths.
			fullPath := fmt.Sprintf("%s/%s", tmpDir, test.FullPath)
			rootPath := fmt.Sprintf("%s/%s", tmpDir, test.RootPath)

			// Create a chain of direcories.
			err := os.MkdirAll(fullPath, os.ModePerm)
			require.NoError(t, err)

			// Prune empty dirs within rootPath.
			err = pruneEmptyDirs(rootPath, true)
			require.NoError(t, err)

			// Ensure rootPath directory still exists.
			expectPath := filepath.Join(tmpDir, "root")
			require.DirExists(t, expectPath)
		})
	}
}

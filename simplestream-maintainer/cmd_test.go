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

	tmpDir := t.TempDir()

	tests := []struct {
		Name          string
		Mock          testutils.ProductMock
		WantErrString string
		WantCatalog   stream.ProductCatalog
		WantIndex     stream.StreamIndex
	}{
		{
			Name: "Ensure empty index and catalog are created",
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/lunar/amd64/cloud"),
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
			Mock: testutils.MockProduct(t, tmpDir, "images-daily/ubuntu/focal/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "disk.qcow2"). // Missing rootfs.squashfs
				AddVersion("2024_01_02", "lxd.tar.xz").               // Incomplete version
				AddVersion("2024_01_03", "lxd.tar.xz", "disk.qcow2", "rootfs.squashfs"),
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

			err := buildIndex(context.Background(), tmpDir, "v1", []string{p.StreamName()}, 2)
			require.NoError(t, err, "Failed building index and catalog files!")

			// Convert expected catalog and index files to json.
			jsonCatalogExpect, err := json.MarshalIndent(test.WantCatalog, "", "  ")
			require.NoError(t, err)

			jsonIndexExpect, err := json.MarshalIndent(test.WantIndex, "", "  ")
			require.NoError(t, err)

			// Read actual catalog and index files.
			catalogPath := filepath.Join(tmpDir, "streams", "v1", fmt.Sprintf("%s.json", p.StreamName()))
			indexPath := filepath.Join(tmpDir, "streams", "v1", "index.json")

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

	// Mimic a product directory structure.
	tmpDir := filepath.Join(t.TempDir(), "images/ubuntu/noble/amd64/cloud")

	checksums := []string{
		// Checksums for content "test-content".
		"0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e  lxd.tar.xz",    //Valid for "test-content".
		"0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e  root.squashfs", //Valid for "test-content".
		"0a_InvalidSHA256Checksum_72beeb5b93124cce1bf3701c9d6cdeb543cb73e  disk.qcow2",    // Invalid checksum.
	}

	tests := []struct {
		Name         string
		Mock         testutils.ProductMock
		WantVersions []string // Map of product id and expected versions.
	}{
		{
			Name: "Ensure checksum validation is ignored when checksum file is missing",
			Mock: func() testutils.ProductMock {
				p := testutils.MockProduct(t, tmpDir, "images-00/ubuntu/noble/amd64/cloud")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_02", "lxd.tar.xz", "root.squashfs")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_03", "lxd.tar.xz", "disk.qcow2")
				return p
			}(),
			WantVersions: []string{
				"2024_01_01",
				"2024_01_02",
				"2024_01_03",
			},
		},
		{
			Name: "Ensure valid versions are included the product catalog.",
			Mock: func() testutils.ProductMock {
				p := testutils.MockProduct(t, tmpDir, "images-01/ubuntu/noble/amd64/cloud")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_01", "lxd.tar.xz", "root.squashfs").SetChecksumFile(checksums...)
				return p
			}(),
			WantVersions: []string{
				"2024_01_01",
			},
		},
		{
			Name: "Ensure versions with mismatched checksums are excluded from the product catalog",
			Mock: func() testutils.ProductMock {
				p := testutils.MockProduct(t, tmpDir, "images-02/ubuntu/noble/amd64/cloud")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").SetChecksumFile(checksums...)
				return p
			}(),
			WantVersions: []string{},
		},
		{
			Name: "Ensure version is excluded if checksum file exists, but checksum for a certain item is missing",
			Mock: func() testutils.ProductMock {
				p := testutils.MockProduct(t, tmpDir, "images-03/ubuntu/noble/amd64/cloud")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_01", "lxd.tar.xz", "root.squashfs", "no-sha.qcow2").SetChecksumFile(checksums...)
				return p
			}(),
			WantVersions: []string{},
		},
		{
			Name: "Ensure version with mismatched checksums is excluded but product catalog is still created",
			Mock: func() testutils.ProductMock {
				p := testutils.MockProduct(t, tmpDir, "images-10/ubuntu/noble/amd64/cloud")
				testutils.MockVersion(t, p.AbsPath(), "2024_01_01", "lxd.tar.xz", "root.squashfs").SetChecksumFile(checksums...)
				// testutils.MockVersion(t, p.AbsPath(), "2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2").SetChecksumFile(checksums...)
				testutils.MockVersion(t, p.AbsPath(), "2024_01_03", "lxd.tar.xz", "root.squashfs").SetChecksumFile(checksums...)
				return p
			}(),
			WantVersions: []string{
				"2024_01_01",
				"2024_01_03",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock

			// Build product catalog.
			catalog, err := buildProductCatalog(context.Background(), tmpDir, "v1", p.StreamName(), 2)
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

// GenFile generates a temporary file of the given size.
func GenFile(t *testing.T, sizeInMB int) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), fmt.Sprint("testfile-", sizeInMB, "MB-"))
	require.NoError(t, err)

	_, err = tmpFile.Write(make([]byte, sizeInMB*1024*1024))
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

func TestPruneOldVersions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

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
			Mock: testutils.MockProduct(t, tmpDir, "test_000/ubuntu/noble/amd64/cloud").
				BuildProductCatalog(),
			KeepVersions: 1,
			WantVersions: []string{},
		},
		{
			Name: "Ensure exact number of versions is kept",
			Mock: testutils.MockProduct(t, tmpDir, "test_010/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_03", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog(),
			KeepVersions: 3,
			WantVersions: []string{
				"2024_01_01",
				"2024_01_02",
				"2024_01_03",
			},
		},
		{
			Name: "Ensure the given number of product versions is retained",
			Mock: testutils.MockProduct(t, tmpDir, "test_020/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_05", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_05_01", "lxd.tar.xz", "root.squashfs").
				AddVersion("2025_01_01", "lxd.tar.xz", "disk.qcow2").
				BuildProductCatalog(),
			KeepVersions: 3,
			WantVersions: []string{
				"2024_01_05",
				"2024_05_01",
				"2025_01_01",
			},
		},
		{
			Name: "Ensure only complete versions are retained",
			Mock: testutils.MockProduct(t, tmpDir, "test_030/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz").
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs").
				AddVersion("2024_01_03", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_04", "root.squashfs").
				AddVersion("2024_01_05", "lxd.tar.xz", "disk.qcow2").
				AddVersion("2024_01_06", "disk.qcow2").
				BuildProductCatalog(),
			KeepVersions: 2,
			WantVersions: []string{
				"2024_01_03",
				"2024_01_05",
			},
		},
		{
			Name: "Ensure only referenced versions are prunned",
			Mock: testutils.MockProduct(t, tmpDir, "test_040/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_03", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog().
				AddVersion("2024_01_04", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_05", "lxd.tar.xz", "root.squashfs", "disk.qcow2"),
			KeepVersions: 2,
			WantVersions: []string{
				"2024_01_02",
				"2024_01_03",
				"2024_01_04",
				"2024_01_05",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock

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
	tmpDir := t.TempDir()

	tests := []struct {
		Name         string
		Mock         testutils.ProductMock
		WantProducts map[string][]string // product: list of versions
	}{
		{
			Name: "Ensure no error on empty product catalog",
			Mock: testutils.MockProduct(t, tmpDir, "test_000/ubuntu/noble/amd64/cloud").
				BuildProductCatalog(),
			WantProducts: map[string][]string{},
		},
		{
			Name: "Ensure referenced product version is not removed",
			Mock: testutils.MockProduct(t, tmpDir, "test_010/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog(),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
				},
			},
		},
		{
			Name: "Ensure referenced product version older then 1 day is not removed",
			Mock: testutils.MockProduct(t, tmpDir, "test_020/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog().
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
				},
			},
		},
		{
			Name: "Ensure fresh unreferenced product version is not removed",
			Mock: testutils.MockProduct(t, tmpDir, "test_030/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog().
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2"),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
					"2024_01_02",
				},
			},
		},
		{
			Name: "Ensure unreferenced old product is removed",
			Mock: testutils.MockProduct(t, tmpDir, "test_040/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				BuildProductCatalog().
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
				},
			},
		},
		{
			Name: "Ensure unreferenced old product is not removed when product catalog is not empty",
			Mock: testutils.MockProduct(t, tmpDir, "test_050/ubuntu/noble/amd64/cloud").
				BuildProductCatalog().
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				SetFilesAge(24 * time.Hour),
			WantProducts: map[string][]string{
				"ubuntu:noble:amd64:cloud": {
					"2024_01_01",
				},
			},
		},
		{
			Name: "Ensure only unreferenced project versions are removed",
			Mock: testutils.MockProduct(t, tmpDir, "test_060/ubuntu/noble/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs").
				BuildProductCatalog().
				AddVersion("2024_01_03", "lxd.tar.xz", "disk.qcow2").
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

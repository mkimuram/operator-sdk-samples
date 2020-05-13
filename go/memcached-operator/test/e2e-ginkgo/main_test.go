package e2e_ginkgo

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	projectRoot        string
	globalManifests    []string
	globalManifestsStr string
	localManifests     []string
	localManifestsStr  string
	image              string
	imagePullPolicy    string
)

func init() {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}

	// Set flags
	flag.StringVar(&projectRoot, "root", filepath.Join(wd, "..", ".."), "path to project root")
	flag.StringVar(&globalManifestsStr, "global-manifest", "", "comma-separated list of global manifest files")
	flag.StringVar(&localManifestsStr, "local-manifest", "", "comma-separated list of namespaced manifest files")
	flag.StringVar(&image, "image", "", "image name of operator")
	flag.StringVar(&imagePullPolicy, "image-pull-policy", "", "image pull policy of operator")
}

func TestMain(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2e Suite")
}

func setVars() {
	// Set globalManifests
	if globalManifestsStr != "" {
		globalManifests = []string{}
		s := strings.Split(globalManifestsStr, ",")
		for _, path := range s {
			if filepath.IsAbs(path) {
				globalManifests = append(globalManifests, path)
			} else {
				globalManifests = append(globalManifests, filepath.Join(projectRoot, path))
			}
		}
	} else {
		globalManifests = []string{filepath.Join(projectRoot, "deploy", "crds")}
	}

	// Set localManifests
	if localManifestsStr != "" {
		localManifests = []string{}
		s := strings.Split(localManifestsStr, ",")
		for _, path := range s {
			if filepath.IsAbs(path) {
				localManifests = append(localManifests, path)
			} else {
				localManifests = append(localManifests, filepath.Join(projectRoot, path))
			}
		}
	} else {
		lManPath := filepath.Join(projectRoot, "deploy")
		localManifests = []string{
			filepath.Join(lManPath, "operator.yaml"),
			filepath.Join(lManPath, "role_binding.yaml"),
			filepath.Join(lManPath, "role.yaml"),
			filepath.Join(lManPath, "service_account.yaml"),
		}
	}
}

var _ = ginkgo.SynchronizedBeforeSuite(
	// Run only on Ginkgo node 1
	func() []byte {
		// setVars needs to be called here to set globalManifests
		setVars()
		err := initCRDs(globalManifests)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to initialize suite")
		return nil
	},
	// Run on all Ginkgo nodes
	func(data []byte) {
		setVars()
	},
)

var _ = ginkgo.SynchronizedAfterSuite(
	// Run on all Ginkgo nodes
	func() {
		// Do nothing
	},
	// Run only on Ginkgo node 1
	func() {
		err := teardownCRDs(globalManifests)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to teardown suite")
	},
)

package e2e_ginkgo

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis"
	operator "github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis/cache/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = ginkgo.Describe("[memcached]", func() {
	var (
		err               error
		cl                client.Client
		conf              *rest.Config
		operatorNamespace string
		watchNamespace    string
		otherNamespace    string
		opt               patchOption
	)
	ginkgo.BeforeEach(func() {
		// Initialize client
		cl, conf, err = initClient([]addToSchemeFunc{apis.AddToScheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to initialize client")

		// Create operator namespace
		operatorNamespace, err = createNamespace(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create operator namespace")

		// Create watch namespace
		watchNamespace, err = createNamespace(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create watch namespace")

		// Create other namespace (Just to test that this name won't affected by operator)
		otherNamespace, err = createNamespace(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create other namespace")

		// Create operator
		opt = patchOption{
			operatorNamespace: operatorNamespace,
			watchNamespace:    watchNamespace,
			image:             image,
			imagePullPolicy:   imagePullPolicy,
		}
		err = initOperator(cl, localManifests, defaultPatchFunc(opt))
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to initialize operator")
	})

	ginkgo.AfterEach(func() {
		// Delete operator
		err = teardownOperator(cl, localManifests, defaultPatchFunc(opt))
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to tear down operator")

		// Delete other namespace
		err = deleteNamespace(cl, otherNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete other namespace")

		// Delete watch namespace
		err = deleteNamespace(cl, watchNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete watch namespace")

		// Delete operator namespace
		err = deleteNamespace(cl, operatorNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete operator namespace")
	})

	ginkgo.It("should scale 3 to 4", func() {
		memcachedScaleTest(cl, watchNamespace, 3, 4)
		fmt.Fprintf(ginkgo.GinkgoWriter, "Operator log: %v\n", getOperatorLog(conf, operatorNamespace))
	})

	ginkgo.It("[slow]should scale 3 to 5", func() {
		memcachedScaleTest(cl, watchNamespace, 3, 5)
	})

	ginkgo.It("should not created in other namespace", func() {
		func() {
			defer ginkgo.GinkgoRecover()
			memcachedNotCreatedTest(cl, otherNamespace, 3)
		}()
		fmt.Fprintf(ginkgo.GinkgoWriter, "Operator log: %v\n", getOperatorLog(conf, operatorNamespace))
	})
})

func memcachedScaleTest(cl client.Client, ns string, origSize, scaleSize int) {
	var (
		retryInterval = time.Second * 5
		timeout       = time.Second * 60
		name          = "example-memcached"
	)
	// create memcached custom resource
	exampleMemcached := &operator.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: operator.MemcachedSpec{
			Size: int32(origSize),
		},
	}
	var err error

	// Create exampleMemcached
	err = cl.Create(context.TODO(), exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create memcahced custom resource")
	defer cl.Delete(context.TODO(), exampleMemcached)

	// wait for example-memcached to reach origSize replicas
	err = waitForDeployment(cl, ns, name, origSize, retryInterval, timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("memcached replica didn't reach to specified size %d", origSize))

	err = cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: ns}, exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get example-memcached")

	exampleMemcached.Spec.Size = int32(scaleSize)
	err = cl.Update(context.TODO(), exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to update example-memcached")

	// wait for example-memcached to reach scaleSize replicas
	err = waitForDeployment(cl, ns, name, scaleSize, retryInterval, timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("memcached replica didn't reach to specified size %d after scale", scaleSize))
}

func memcachedNotCreatedTest(cl client.Client, ns string, size int) {
	var (
		retryInterval = time.Second * 5
		timeout       = time.Second * 60
		name          = "example-memcached"
	)
	// create memcached custom resource
	exampleMemcached := &operator.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: operator.MemcachedSpec{
			Size: int32(size),
		},
	}
	var err error

	// Create exampleMemcached
	err = cl.Create(context.TODO(), exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create memcahced custom resource")
	defer cl.Delete(context.TODO(), exampleMemcached)

	// wait for example-memcached to reach origSize replicas
	err = waitForDeployment(cl, ns, name, size, retryInterval, timeout)
	gomega.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("memcached replica shouldn't reach to specified size %d", size))
}

// Just for debug
func getOperatorLog(conf *rest.Config, ns string) string {
	kcl, err := kubernetes.NewForConfig(conf)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create client")

	options := metav1.ListOptions{
		LabelSelector: "name=memcached-operator",
	}
	podList, err := kcl.CoreV1().Pods(ns).List(options)
	gomega.Expect(len(podList.Items)).To(gomega.Equal(1), "Number of operator pod is not 1")
	pod := podList.Items[0]

	closer, err := kcl.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "memcached-operator"}).Stream()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get log")
	defer closer.Close()
	buf := new(strings.Builder)
	_, err = io.Copy(buf, closer)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to copy log")

	return fmt.Sprintf("pod:%#v\nlog:%s", pod, buf.String())
}

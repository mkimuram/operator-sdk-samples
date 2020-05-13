package e2e_ginkgo

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis"
	operator "github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis/cache/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = ginkgo.Describe("[memcached]", func() {
	var (
		err error
		cl  client.Client
		ns  string
		opt patchOption
	)
	ginkgo.BeforeEach(func() {
		// Initialize client
		cl, err = initClient([]addToSchemeFunc{apis.AddToScheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to initialize client")

		// Create namespace
		ns, err = createNamespace(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create namespace")

		// Create operator
		opt = patchOption{
			operatorNamespace: ns,
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

		// Delete namespace
		err = deleteNamespace(cl, ns)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete namespace")
	})

	ginkgo.It("should scale 3 to 4", func() {
		memcachedScaleTest(cl, ns, 3, 4)
	})

	ginkgo.It("[slow]should scale 3 to 5", func() {
		memcachedScaleTest(cl, ns, 3, 5)
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

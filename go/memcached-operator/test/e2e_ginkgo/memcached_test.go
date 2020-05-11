// Copyright 2020 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e_ginkgo

import (
	goctx "context"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	operator "github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis/cache/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = ginkgo.Describe("[memcached]", func() {
	var (
		ctx *framework.TestCtx
		f   *framework.Framework
		ns  string
	)
	ginkgo.BeforeEach(func() {
		f = framework.Global
		ctx, ns = initOperator(operatorName, objs)
	})

	ginkgo.AfterEach(func() {
		ctx.Cleanup()
	})

	ginkgo.It("should scale 3 to 4", func() {
		memcachedScaleTest(f, ctx, ns, 3, 4)
	})

	ginkgo.It("[slow]should scale 3 to 5", func() {
		memcachedScaleTest(f, ctx, ns, 3, 5)
	})

})

func memcachedScaleTest(f *framework.Framework, ctx *framework.TestCtx, ns string, origSize, scaleSize int) {
	// create memcached custom resource
	exampleMemcached := &operator.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-memcached",
			Namespace: ns,
		},
		Spec: operator.MemcachedSpec{
			Size: int32(origSize),
		},
	}
	var err error

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), exampleMemcached, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to create memcahced custom resource")

	// wait for example-memcached to reach origSize replicas
	err = e2eutil.WaitForDeployment(nil, f.KubeClient, ns, "example-memcached", origSize, retryInterval, timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("memcached replica didn't reach to specified size %d", origSize))

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-memcached", Namespace: ns}, exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get example-memcached")

	exampleMemcached.Spec.Size = int32(scaleSize)
	err = f.Client.Update(goctx.TODO(), exampleMemcached)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to update example-memcached")

	// wait for example-memcached to reach scaleSize replicas
	err = e2eutil.WaitForDeployment(nil, f.KubeClient, ns, "example-memcached", scaleSize, retryInterval, timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("memcached replica didn't reach to specified size %d after scale", scaleSize))
}

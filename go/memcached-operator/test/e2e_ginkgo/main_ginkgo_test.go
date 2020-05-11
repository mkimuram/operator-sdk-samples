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
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis"
	operator "github.com/operator-framework/operator-sdk-samples/go/memcached-operator/pkg/apis/cache/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	operatorName = "memcached-operator"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 60
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5

	objs = []runtime.Object{&operator.MemcachedList{}}
)

func TestGinkgo(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2e Suite")
}

func initOperator(name string, objs []runtime.Object) (*framework.TestCtx, string) {
	for _, obj := range objs {
		err := framework.AddToFrameworkScheme(apis.AddToScheme, obj)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to add custom resource scheme to framework")
	}

	ctx := framework.NewTestCtx(nil)
	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to initialize cluster resources")

	fmt.Fprintf(ginkgo.GinkgoWriter, "Initialized cluster resources\n")

	namespace, err := ctx.GetOperatorNamespace()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get namespace")

	// get global framework variables
	f := framework.Global
	// wait for memcached-operator to be ready
	err = e2eutil.WaitForOperatorDeployment(nil, f.KubeClient, namespace, name, 1, retryInterval, timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "operator failed to be ready")

	return ctx, namespace
}

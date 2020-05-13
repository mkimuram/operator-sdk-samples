package e2e_ginkgo

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pborman/uuid"
	logger "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	extscheme "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	cgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type addToSchemeFunc func(*runtime.Scheme) error

func getKubeconfig() (*rest.Config, error) {
	var kubeconfigPath string
	kcFlag := flag.Lookup("kubeconfig")
	if kcFlag != nil {
		kubeconfigPath = kcFlag.Value.String()
	}
	conf, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	return conf, nil
}

func getScheme(addToSchemes []addToSchemeFunc) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	if err := cgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add k8s api to scheme: %v", err)
	}

	if err := extscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add api extensions to shceme: %v", err)
	}

	for _, f := range addToSchemes {
		if err := f(scheme); err != nil {
			return nil, fmt.Errorf("failed to add operator api to shceme: %v", err)
		}
	}

	return scheme, nil
}

func createNamespace(cl client.Client) (string, error) {
	// Create namespace
	ns := "osdk-e2e-" + uuid.New()

	nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
	if err := cl.Create(context.TODO(), nsObj); err != nil {
		return "", fmt.Errorf("failed to create namespace: %v", err)
	}

	return ns, nil
}

func deleteNamespace(cl client.Client, ns string) error {
	nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
	if err := cl.Delete(context.TODO(), nsObj); err != nil {
		return fmt.Errorf("faied to delete namespace: %v", err)
	}

	return nil
}

func initCRDs(manifests []string) error {
	// Get kubeconfig
	conf, err := getKubeconfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	// Install CRDs in globalManifests
	_, err = envtest.InstallCRDs(conf, envtest.CRDInstallOptions{Paths: manifests, ErrorIfPathMissing: true})
	if err != nil {
		return fmt.Errorf("failed to install CRDs: %v", err)
	}

	return nil
}

func teardownCRDs(manifests []string) error {
	// Get kubeconfig
	conf, err := getKubeconfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	// Uninstall CRDs in globalManifests
	err = envtest.UninstallCRDs(conf, envtest.CRDInstallOptions{Paths: manifests})
	if err != nil {
		return fmt.Errorf("failed to uninstall CRDs: %v", err)
	}

	return nil
}

func initClient(addToSchemes []addToSchemeFunc) (client.Client, error) {
	// Get kubeconfig
	conf, err := getKubeconfig()
	if err != nil {
		return nil, err
	}
	// Get Scheme
	scheme, err := getScheme(addToSchemes)
	if err != nil {
		return nil, err
	}

	// Create dynamic client
	cl, err := client.New(conf, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	return cl, nil
}

func initOperator(cl client.Client, manifests []string, patch func(*unstructured.Unstructured) error) error {
	// Create resource from manifests
	for _, file := range manifests {
		if err := createFromYaml(cl, file, patch); err != nil {
			return fmt.Errorf("failed to create resource from %s: %v", file, err)
		}
	}

	// TODO: consider waiting for operator deployment to be up and running
	return nil
}

func teardownOperator(cl client.Client, manifests []string, patch func(*unstructured.Unstructured) error) error {
	// Delete manifests
	var errStr string
	for _, file := range manifests {
		if err := deleteFromYaml(cl, file, patch); err != nil {
			errStr += fmt.Sprintf("failed to delete resource from %s: %v,", file, err)
		}
	}
	if errStr != "" {
		return fmt.Errorf("%s", errStr)
	}

	return nil
}

func waitForDeployment(cl client.Client, namespace, name string, replicas int, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		deployment := &appsv1.Deployment{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, deployment); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Infof("Waiting for availability of %s deployment\n", name)
				return false, nil
			}
			return false, err
		}

		if int(deployment.Status.AvailableReplicas) == replicas {
			return true, nil
		}
		logger.Infof("Waiting for full availability of %s deployment (%d/%d)\n", name, deployment.Status.AvailableReplicas, replicas)
		return false, nil
	})
	if err != nil {
		return err
	}
	logger.Infof("Deployment available (%d/%d)\n", replicas, replicas)
	return nil
}

type readFunc func() (*unstructured.Unstructured, error)
type patchFunc func(*unstructured.Unstructured) error
type handleFunc func(*unstructured.Unstructured) error

func createFromYaml(cl client.Client, file string, patch patchFunc) error {
	return handleManifest(readFromYamlFunc(file), patch, defaultCreateFunc(cl))
}

func deleteFromYaml(cl client.Client, file string, patch patchFunc) error {
	return handleManifest(readFromYamlFunc(file), patch, defaultDeleteFunc(cl))
}

func handleManifest(
	read readFunc,
	patch patchFunc,
	handle handleFunc,
) error {
	// Read to obj
	obj, err := read()
	if err != nil {
		return err
	}

	// Patch obj
	if patch != nil {
		if err := patch(obj); err != nil {
			return err
		}
	}

	// Create obj
	if err := handle(obj); err != nil {
		return err
	}

	return nil
}

func getFromManifest(
	read readFunc,
	patch patchFunc,
) (*unstructured.Unstructured, error) {
	// Read to obj
	obj, err := read()
	if err != nil {
		return nil, err
	}

	// Patch obj
	if patch != nil {
		if err := patch(obj); err != nil {
			return nil, err
		}
	}

	return obj, nil
}

func readFromYamlFunc(file string) readFunc {
	return func() (*unstructured.Unstructured, error) {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		// Read yaml file
		reader := k8syaml.NewYAMLReader(bufio.NewReader(f))
		yamlSpec, err := reader.Read()
		if err != nil {
			return nil, err
		}

		// Convert yaml to obj
		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return nil, fmt.Errorf("could not convert yaml file to json: %w", err)
		}
		if err := obj.UnmarshalJSON(jsonSpec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal object spec: %w", err)
		}

		return obj, nil
	}
}

type patchOption struct {
	operatorNamespace string
	image             string
	imagePullPolicy   string
}

func defaultPatchFunc(opt patchOption) patchFunc {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		// Non-namespaced resources
		case "ClusterRoleBinding":
			// rename non-namespaced resource to avoid conflict
			origName := obj.GetName()
			obj.SetName(origName + opt.operatorNamespace)
			// modify namespace in subjects to operatorNamespace
			if subjs, ok := obj.Object["subjects"].([]interface{}); ok {
				for _, s := range subjs {
					if subj, ok := s.(map[string]interface{}); ok {
						if _, ok := subj["namespace"].(string); ok {
							subj["namespace"] = opt.operatorNamespace
						}
					}
				}
			}
			// modify name in roleRef to renamed name if kind is ClusterRole
			if roleRef, ok := obj.Object["roleRef"].(map[string]interface{}); ok {
				if kind, ok := roleRef["kind"].(string); ok {
					if kind == "ClusterRole" {
						if origName, ok := roleRef["name"].(string); ok {
							roleRef["name"] = origName + opt.operatorNamespace
						}
					}
				}
			}
		case "ClusterRole":
			// rename non-namespaced resource to avoid conflict
			origName := obj.GetName()
			obj.SetName(origName + opt.operatorNamespace)

		// Namespaced resources
		case "ServiceAccount", "Role", "RoleBinding":
			// Modify namespace
			obj.SetNamespace(opt.operatorNamespace)
		case "Deployment":
			// Modify namespace
			obj.SetNamespace(opt.operatorNamespace)

			// Decode into deploy object
			data, err := obj.MarshalJSON()
			if err != nil {
				return err
			}
			deploy := &appsv1.Deployment{}
			if err := runtime.DecodeInto(cgoscheme.Codecs.UniversalDecoder(), data, deploy); err != nil {
				return err
			}

			// Modify deployment
			containers := []corev1.Container{}
			for _, container := range deploy.Spec.Template.Spec.Containers {
				// Change image
				if opt.image != "" {
					container.Image = opt.image
				}
				// Change image pull policy
				if opt.imagePullPolicy != "" {
					container.ImagePullPolicy = corev1.PullPolicy(opt.imagePullPolicy)
				}
				containers = append(containers, container)
			}
			deploy.Spec.Template.Spec.Containers = containers

			// Change back to unstructured
			jsonSpec, err := json.Marshal(deploy)
			if err != nil {
				return err
			}
			newObj := &unstructured.Unstructured{}
			if err := newObj.UnmarshalJSON(jsonSpec); err != nil {
				return err
			}

			// Deepcopy new object into original object
			newObj.DeepCopyInto(obj)
		default:
			return fmt.Errorf("Unexpected resource kind %s was passed", obj.GetKind())
		}

		return nil
	}
}

func defaultCreateFunc(cl client.Client) handleFunc {
	return func(obj *unstructured.Unstructured) error {
		if err := cl.Create(context.TODO(), obj); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}
		return nil
	}
}

func defaultDeleteFunc(cl client.Client) handleFunc {
	return func(obj *unstructured.Unstructured) error {
		if err := cl.Delete(context.TODO(), obj); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	}
}

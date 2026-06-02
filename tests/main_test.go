package tests

import (
	"path/filepath"
	osRuntime "runtime"
	"testing"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	formanceGroupVersion = schema.GroupVersion{Group: "formance.com", Version: "v1beta1"}

	testEnv    *envtest.Environment
	restConfig *rest.Config
	k8sClient  *rest.RESTClient
	mapper     meta.RESTMapper
)

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(5 * time.Second)

	_, filename, _, _ := osRuntime.Caller(0)

	apiServer := envtest.APIServer{}
	apiServer.Configure().
		Set("service-cluster-ip-range", "10.0.0.0/20")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(filepath.Dir(filename), "..", "dist", "operator",
				"config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &apiServer,
		},
	}

	var err error
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	restConfig.GroupVersion = &formanceGroupVersion
	restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restConfig.APIPath = "/apis"

	k8sClient, err = rest.RESTClientFor(restConfig)
	Expect(err).NotTo(HaveOccurred())

	mapper, err = internal.CreateRestMapper(restConfig, logging.Testing())
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	if testEnv != nil {
		Expect(testEnv.Stop()).To(BeNil())
	}
})

func TestAgent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Suite")
}

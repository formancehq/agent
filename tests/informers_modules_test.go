package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Informer modules", func() {
	var (
		membershipClientMock *internal.MembershipClientMock
	)

	BeforeEach(func() {
		membershipClientMock = internal.NewMembershipClientMock()
	})
	When("a module is created on the cluster", func() {
		var (
			modules map[schema.GroupVersionKind]*unstructured.Unstructured
		)
		BeforeEach(func() {
			modules = map[schema.GroupVersionKind]*unstructured.Unstructured{}

			moduleCRDs, _, err := internal.RetrieveModuleList(context.Background(), restConfig)
			Expect(err).ToNot(HaveOccurred())

			for _, crd := range moduleCRDs {
				kind := crd.Spec.Names.Kind
				resource := crd.Status.AcceptedNames.Plural
				version := crd.Spec.Versions[0].Name
				gvk := schema.GroupVersionKind{
					Group:   crd.Spec.Group,
					Version: version,
					Kind:    kind,
				}

				name := uuid.NewString()

				unstructuredObj := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": gvk.GroupVersion().String(),
						"kind":       kind,
						"metadata": map[string]interface{}{
							"name": name,
							"labels": map[string]interface{}{
								"formance.com/stack":            name,
								"formance.com/created-by-agent": "true",
							},
						},
					},
				}

				By(fmt.Sprintf("Creating the module %s", kind), func() {
					Expect(k8sClient.Post().
						Resource(resource).
						Body(unstructuredObj).
						Name(name).Do(context.Background()).Error()).To(Succeed())

					DeferCleanup(func() {
						Expect(client.IgnoreNotFound(k8sClient.Delete().Resource(resource).Name(name).Do(context.Background()).Error())).To(Succeed())
					})
				})

				By(fmt.Sprintf("Loading then updating status %s", kind), func() {
					Eventually(func() error {
						return LoadResource(resource, name, unstructuredObj)
					}).Should(Succeed())

					// Those 2 are reset by LoadResource to empty
					unstructuredObj.Object["apiVersion"] = gvk.GroupVersion().String()
					unstructuredObj.Object["kind"] = kind

					unstructuredObj.Object["status"] = map[string]interface{}{
						"info": uuid.NewString(),
					}

					Expect(k8sClient.Put().
						Resource(resource).
						SubResource("status").
						Name(name).
						Body(unstructuredObj).
						Do(context.Background()).
						Error()).To(Succeed())
				})

				modules[gvk] = unstructuredObj
			}
		})
		It("Should have been created", func() {
			moduleCRDs, _, err := internal.RetrieveModuleList(context.Background(), restConfig)
			Expect(err).ToNot(HaveOccurred())

			for _, crd := range moduleCRDs {
				version := crd.Spec.Versions[0].Name
				gvk := schema.GroupVersionKind{
					Group:   crd.Spec.Group,
					Version: version,
					Kind:    crd.Spec.Names.Kind,
				}
				resource := crd.Status.AcceptedNames.Plural

				Expect(true).To(BeTrue())
				Eventually(func() error {
					return LoadResource(resource, modules[gvk].GetName(), modules[gvk])
				}).Should(Succeed())
			}
		})
		When("Listening to all modules", func() {
			BeforeEach(func() {
				dynamicClient, err := dynamic.NewForConfig(restConfig)
				Expect(err).ToNot(HaveOccurred())
				factory := internal.NewDynamicSharedInformerFactory(dynamicClient, 5*time.Minute)

				moduleCRDs, _, err := internal.RetrieveModuleList(context.Background(), restConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(internal.CreateModulesInformers(factory, moduleCRDs, logging.Testing(), membershipClientMock)).ToNot(HaveOccurred())

				stopCh := make(chan struct{})
				factory.Start(stopCh)
				DeferCleanup(func() {
					close(stopCh)
				})
			})
			It("Should have sent ModuleStatusChanged", func() {
				for gvk, module := range modules {
					By(fmt.Sprintf("Checking the module %s", gvk.Kind), func() {
						Eventually(func(g Gomega) bool {
							for _, message := range membershipClientMock.GetMessages() {
								if msg := message.GetModuleStatusChanged(); msg != nil &&
									msg.Vk.Kind == gvk.Kind &&
									msg.Vk.Version == gvk.Version &&
									msg.ClusterName == module.GetName() {

									status, _, _ := unstructured.NestedMap(module.Object, "status")

									g.Expect(msg.Status.AsMap()["info"]).ToNot(BeNil())
									g.Expect(msg.Status.AsMap()["ready"]).To(BeFalse())

									for k, value := range status {
										g.Expect(msg.Status.AsMap()[k]).To(Equal(value))
									}
									return true
								}
							}
							return false
						}).Should(BeTrue())
					})
				}
			})
			When("A module is deleted", func() {
				var moduleDeleted *unstructured.Unstructured
				BeforeEach(func() {
					moduleCRDs, _, err := internal.RetrieveModuleList(context.Background(), restConfig)
					Expect(err).ToNot(HaveOccurred())

					for _, crd := range moduleCRDs {
						version := crd.Spec.Versions[0].Name
						gvk := schema.GroupVersionKind{
							Group:   crd.Spec.Group,
							Version: version,
							Kind:    crd.Spec.Names.Kind,
						}
						module, ok := modules[gvk]
						if !ok {
							continue
						}
						By(fmt.Sprintf("Deleting the module %s", gvk.Kind), func() {
							moduleDeleted = module
							resource := crd.Status.AcceptedNames.Plural
							Expect(k8sClient.Delete().Resource(resource).Name(module.GetName()).Do(context.Background()).Error()).To(Succeed())
						})
					}
				})
				It("Should have sent ModuleDeleted", func() {
					By(fmt.Sprintf("Checking message received for %s", moduleDeleted.GetKind()), func() {
						Eventually(func(g Gomega) bool {
							for _, message := range membershipClientMock.GetMessages() {
								if msg := message.GetModuleDeleted(); msg != nil &&
									msg.Vk.Kind == moduleDeleted.GetKind() &&
									msg.Vk.Version == strings.Split(moduleDeleted.GetAPIVersion(), "/")[1] &&
									msg.ClusterName == moduleDeleted.GetName() {
									return true
								}
							}
							return false
						}).Should(BeTrue())
					})
				})
			})
		})
	})
})

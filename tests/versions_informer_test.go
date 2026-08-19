package tests

import (
	"context"
	"encoding/json"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

var _ = Describe("Versions informer", func() {
	var (
		membershipClientMock *internal.MembershipClientMock
		startListener        func()
	)
	BeforeEach(func() {
		membershipClientMock = internal.NewMembershipClientMock()
		dynamicClient, err := dynamic.NewForConfig(restConfig)
		Expect(err).To(Succeed())

		factory := internal.NewDynamicSharedInformerFactory(dynamicClient, 5*time.Minute)
		Expect(internal.CreateVersionsInformer(factory, logging.Testing(), membershipClientMock)).To(Succeed())
		startListener = func() {
			stopCh := make(chan struct{})
			factory.Start(stopCh)
			DeferCleanup(func() {
				close(stopCh)
			})
		}
	})
	When("a Versions resource is created", func() {
		var version *unstructured.Unstructured
		BeforeEach(func() {
			version = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": formanceGroupVersion.String(),
					"kind":       "Versions",
					"metadata": map[string]interface{}{
						"name": uuid.NewString(),
						"annotations": map[string]interface{}{
							"formance.com/deprecated": "true",
						},
					},
					"spec": map[string]interface{}{
						"ledger":   "v2.0.0",
						"payments": "v1.5.0",
					},
				},
			}
			Expect(k8sClient.Post().
				Resource("Versions").
				Body(version).
				Do(context.Background()).
				Into(version)).To(Succeed())

			startListener()

			DeferCleanup(func() {
				k8sClient.Delete().
					Resource("Versions").
					Name(version.GetName()).
					Do(context.Background())
			})
		})
		It("Should send AddedVersion with spec and deprecated flag", func() {
			Eventually(func(g Gomega) {
				for _, message := range membershipClientMock.GetMessages() {
					if msg := message.GetAddedVersion(); msg != nil && msg.Name == version.GetName() {
						g.Expect(msg.Versions["ledger"]).To(Equal("v2.0.0"))
						g.Expect(msg.Versions["payments"]).To(Equal("v1.5.0"))
						g.Expect(msg.Deprecated).To(BeTrue())
						return
					}
				}
				g.Expect(false).To(BeTrue(), "AddedVersion message not found")
			}).Should(Succeed())
		})
		When("the spec is updated", func() {
			BeforeEach(func() {
				// Wait for AddedVersion first
				Eventually(func() bool {
					for _, message := range membershipClientMock.GetMessages() {
						if msg := message.GetAddedVersion(); msg != nil && msg.Name == version.GetName() {
							return true
						}
					}
					return false
				}).Should(BeTrue())

				patch, err := json.Marshal(map[string]any{
					"spec": map[string]any{
						"ledger":   "v2.1.0",
						"payments": "v1.5.0",
					},
				})
				Expect(err).To(BeNil())
				Expect(k8sClient.Patch(types.MergePatchType).
					Resource("Versions").
					Name(version.GetName()).
					Body(patch).
					Do(context.Background()).
					Error()).To(Succeed())
			})
			It("Should send UpdatedVersion", func() {
				Eventually(func(g Gomega) {
					for _, message := range membershipClientMock.GetMessages() {
						if msg := message.GetUpdatedVersion(); msg != nil && msg.Name == version.GetName() {
							g.Expect(msg.Versions["ledger"]).To(Equal("v2.1.0"))
							return
						}
					}
					g.Expect(false).To(BeTrue(), "UpdatedVersion message not found")
				}).Should(Succeed())
			})
		})
		When("the resource is deleted", func() {
			BeforeEach(func() {
				// Wait for AddedVersion first
				Eventually(func() bool {
					for _, message := range membershipClientMock.GetMessages() {
						if msg := message.GetAddedVersion(); msg != nil && msg.Name == version.GetName() {
							return true
						}
					}
					return false
				}).Should(BeTrue())

				Expect(k8sClient.Delete().
					Resource("Versions").
					Name(version.GetName()).
					Do(context.Background()).Error()).To(Succeed())
			})
			It("Should send DeletedVersion", func() {
				Eventually(func(g Gomega) {
					for _, message := range membershipClientMock.GetMessages() {
						if msg := message.GetDeletedVersion(); msg != nil && msg.Name == version.GetName() {
							return
						}
					}
					g.Expect(false).To(BeTrue(), "DeletedVersion message not found")
				}).Should(Succeed())
			})
		})
	})
	When("a Versions resource has no spec", func() {
		var version *unstructured.Unstructured
		BeforeEach(func() {
			version = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": formanceGroupVersion.String(),
					"kind":       "Versions",
					"metadata": map[string]interface{}{
						"name": uuid.NewString(),
					},
				},
			}
			Expect(k8sClient.Post().
				Resource("Versions").
				Body(version).
				Do(context.Background()).
				Into(version)).To(Succeed())

			startListener()

			DeferCleanup(func() {
				k8sClient.Delete().
					Resource("Versions").
					Name(version.GetName()).
					Do(context.Background())
			})
		})
		It("Should send AddedVersion with nil versions", func() {
			Eventually(func(g Gomega) {
				for _, message := range membershipClientMock.GetMessages() {
					if msg := message.GetAddedVersion(); msg != nil && msg.Name == version.GetName() {
						g.Expect(msg.Versions).To(BeEmpty())
						g.Expect(msg.Deprecated).To(BeFalse())
						return
					}
				}
				g.Expect(false).To(BeTrue(), "AddedVersion message not found")
			}).Should(Succeed())
		})
	})
})

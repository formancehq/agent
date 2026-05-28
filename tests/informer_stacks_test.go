package tests

import (
	"context"
	"encoding/json"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

func newStack(name string, disabled bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": formanceGroupVersion.String(),
			"kind":       "Stack",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"disabled": disabled,
			},
		},
	}
	return obj
}

func patchStackStatus(name string, ready bool) {
	patch, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"ready": ready,
		},
	})
	Expect(err).To(Succeed())
	Expect(k8sClient.Patch(types.MergePatchType).
		Resource("Stacks").
		SubResource("status").
		Name(name).
		Body(patch).
		Do(context.Background()).
		Error()).To(Succeed())
}

func patchStackSpec(name string, disabled bool) {
	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"disabled": disabled,
		},
	})
	Expect(err).To(Succeed())
	Expect(k8sClient.Patch(types.MergePatchType).
		Resource("Stacks").
		Name(name).
		Body(patch).
		Do(context.Background()).
		Error()).To(Succeed())
}

var _ = Describe("Stacks informer", func() {
	var (
		membershipClientMock *internal.MembershipClientMock
		startListener        func()
	)
	BeforeEach(func() {
		membershipClientMock = internal.NewMembershipClientMock()
		dynamicClient, err := dynamic.NewForConfig(restConfig)
		Expect(err).To(Succeed())

		factory := internal.NewDynamicSharedInformerFactory(dynamicClient, 5*time.Minute)
		Expect(internal.CreateStacksInformer(factory, logging.Testing(), membershipClientMock)).To(Succeed())
		startListener = func() {
			stopCh := make(chan struct{})
			factory.Start(stopCh)
			DeferCleanup(func() {
				close(stopCh)
			})
		}
	})
	When("a stack is created on the cluster disabled", func() {
		var stack *unstructured.Unstructured
		BeforeEach(func() {
			stack = newStack(uuid.NewString(), true)

			By("Creating a disabled stack", func() {
				Expect(k8sClient.Post().
					Resource("Stacks").
					Body(stack).
					Do(context.Background()).
					Into(stack)).To(Succeed())
			})

			By("Adding ready", func() {
				patchStackStatus(stack.GetName(), true)
			})

			startListener()

			DeferCleanup(func() {
				Expect(k8sClient.Delete().
					Resource("Stacks").
					Name(stack.GetName()).
					Do(context.Background()).Error()).To(Succeed())
			})
		})
		It("Should have sent a Status_Changed", func() {
			Eventually(func() []*generated.Message {
				for _, message := range membershipClientMock.GetMessages() {
					if message.GetStatusChanged() != nil {
						if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
							isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
							if isReady {
								return membershipClientMock.GetMessages()
							}
						}
					}
				}
				return nil
			}).ShouldNot(BeEmpty())
		})
		When("The stack is ready", func() {
			BeforeEach(func() {
				patchStackStatus(stack.GetName(), true)
			})
			It("Should have sent a Status_Changed", func() {
				Eventually(func() []*generated.Message {
					for _, message := range membershipClientMock.GetMessages() {
						if message.GetStatusChanged() != nil {
							if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
								isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
								if isReady {
									return membershipClientMock.GetMessages()
								}
							}
						}
					}
					return nil
				}).ShouldNot(BeEmpty())
			})
		})
		When("the stack is re-enabled", func() {
			BeforeEach(func() {
				By("setting the status ready", func() {
					patchStackStatus(stack.GetName(), false)
				})
				By("Enabling the stack", func() {
					patchStackSpec(stack.GetName(), false)
				})
			})
			It("should have sent a Status_Changed", func() {
				Eventually(func() []*generated.Message {
					for _, message := range membershipClientMock.GetMessages() {
						if message.GetStatusChanged() != nil {
							if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
								isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
								if !isReady {
									return membershipClientMock.GetMessages()
								}
							}
						}
					}
					return []*generated.Message{}
				}).ShouldNot(BeEmpty())
			})
			When("the stack is reconcilled", func() {
				BeforeEach(func() {
					By("Setting the status ready", func() {
						patchStackStatus(stack.GetName(), true)
					})
				})
				It("should have sent a Status_Changed", func() {
					Eventually(func() []*generated.Message {
						for _, message := range membershipClientMock.GetMessages() {
							if message.GetStatusChanged() != nil {
								if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
									isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
									if isReady {
										return membershipClientMock.GetMessages()
									}
								}
							}
						}
						return nil
					}).ShouldNot(BeEmpty())
				})
			})
		})

	})
	When("Stack is created", func() {
		var stack *unstructured.Unstructured
		BeforeEach(func() {
			stack = newStack(uuid.NewString(), false)
			Expect(k8sClient.Post().
				Resource("Stacks").
				Body(stack).
				Do(context.Background()).
				Into(stack)).To(Succeed())

			By("Disabling the status ready", func() {
				patchStackStatus(stack.GetName(), false)
			})

			startListener()
		})
		AfterEach(func() {
			Expect(k8sClient.Delete().
				Resource("Stacks").
				Name(stack.GetName()).
				Do(context.Background()).Error()).To(Succeed())
		})
		It("should have sent a Status_Changed", func() {
			Eventually(func() []*generated.Message {
				for _, message := range membershipClientMock.GetMessages() {
					if message.GetStatusChanged() != nil && message.GetStatusChanged().Status == generated.StackStatus_Progressing && stack.GetName() == message.GetStatusChanged().ClusterName {
						if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
							isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
							if !isReady {
								return membershipClientMock.GetMessages()
							}
						}
					}
				}
				return nil
			}).ShouldNot(BeEmpty())
		})
		When("all stack dependent are ready", func() {
			BeforeEach(func() {
				By("setting the status ready", func() {
					patchStackStatus(stack.GetName(), true)
				})
			})
			It("should have sent a Status_Ready", func() {
				Eventually(func() []*generated.Message {
					for _, message := range membershipClientMock.GetMessages() {
						if message.GetStatusChanged() != nil {
							if _, ok := message.GetStatusChanged().GetStatuses().Fields["ready"]; ok {
								isReady := message.GetStatusChanged().GetStatuses().Fields["ready"].GetBoolValue()
								if isReady {
									return membershipClientMock.GetMessages()
								}
							}
						}
					}
					return nil
				}).ShouldNot(BeEmpty())
			})
		})
	})
	When("Stack is deleted", func() {
		var stack *unstructured.Unstructured
		BeforeEach(func() {
			stack = newStack(uuid.NewString(), false)
			Expect(k8sClient.Post().
				Resource("Stacks").
				Body(stack).
				Do(context.Background()).
				Into(stack)).To(Succeed())

			startListener()

			Expect(k8sClient.Delete().
				Resource("Stacks").
				Name(stack.GetName()).
				Do(context.Background()).Error()).To(Succeed())
		})
		It("should have sent a Stack_Deleted", func() {
			Eventually(func() []*generated.Message {
				for _, message := range membershipClientMock.GetMessages() {
					if message.GetStackDeleted() != nil && message.GetStackDeleted().ClusterName == stack.GetName() {
						return membershipClientMock.GetMessages()
					}
				}
				return nil
			}).ShouldNot(BeEmpty())
		})
	})
})

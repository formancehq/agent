package internal

import (
	"context"
	"reflect"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func versionKindFromUnstructured(u *unstructured.Unstructured) *generated.VersionKind {
	return &generated.VersionKind{
		Version: u.GetObjectKind().GroupVersionKind().Version,
		Kind:    u.GetObjectKind().GroupVersionKind().Kind,
	}
}

type ModuleEventHandler struct {
	logger   logging.Logger
	reporter MembershipReporter
}

func (h *ModuleEventHandler) sendModuleStatus(clusterName string, vk *generated.VersionKind, status *structpb.Struct) error {
	if err := h.reporter.ReportModuleStatus(context.Background(), clusterName, vk, status); err != nil {
		h.logger.Errorf("Unable to send module status to server: %s", err)
		return err
	}
	return nil
}

func (h *ModuleEventHandler) AddFunc(obj interface{}) {
	unstructuredModule := obj.(*unstructured.Unstructured)

	logger := h.logger.WithField("func", "Add").WithField("module", unstructuredModule.GetName())

	status, err := getStatus(unstructuredModule)
	if err != nil {
		logger.Errorf("Unable to generate message module added: %s", err)
		return
	}

	if status == nil {
		return
	}

	vk := versionKindFromUnstructured(unstructuredModule)
	if err := h.sendModuleStatus(unstructuredModule.GetName(), vk, status); err != nil {
		logger.Errorf("Unable to send message module added: %s", err)
		return
	}

	logger.Infof("Detect module '%s' added", unstructuredModule.GetName())
}

func (h *ModuleEventHandler) UpdateFunc(oldObj, newObj any) {

	oldVersions := oldObj.(*unstructured.Unstructured)
	newVersions := newObj.(*unstructured.Unstructured)

	logger := h.logger.WithField("func", "Update").WithField("module", newVersions.GetName())

	oldStatus, err := getStatus(oldVersions)
	if err != nil {
		logger.Errorf("Unable to get status from old versions: %s", err)
	}
	newStatus, err := getStatus(newVersions)
	if err != nil {
		logger.Errorf("Unable to get status from new versions: %s", err)
		return
	}

	if newStatus == nil || reflect.DeepEqual(oldStatus, newStatus) {
		return
	}

	vk := versionKindFromUnstructured(newVersions)
	if err := h.sendModuleStatus(newVersions.GetName(), vk, newStatus); err != nil {
		logger.Errorf("Unable to send message module update: %s", err)
		return
	}
	logger.Infof("Detect module '%s' updated", newVersions.GetName())
}

func (h *ModuleEventHandler) DeleteFunc(obj interface{}) {

	unstructuredModule := obj.(*unstructured.Unstructured)
	logger := h.logger.WithField("func", "Delete").WithField("module", unstructuredModule.GetName())

	vk := versionKindFromUnstructured(unstructuredModule)
	if err := h.reporter.ReportModuleDeleted(context.Background(), unstructuredModule.GetName(), vk); err != nil {
		logger.Errorf("Unable to send message module deleted: %s", err)
		return
	}
	logger.Infof("Detect module '%s' deleted", unstructuredModule.GetName())
}

func NewModuleEventHandler(logger logging.Logger, reporter MembershipReporter) cache.ResourceEventHandlerFuncs {
	moduleEventHandler := &ModuleEventHandler{
		logger:   logger,
		reporter: reporter,
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc:    moduleEventHandler.AddFunc,
		UpdateFunc: moduleEventHandler.UpdateFunc,
		DeleteFunc: moduleEventHandler.DeleteFunc,
	}
}

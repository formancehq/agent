package internal

import (
	"context"
	"reflect"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/operator/v3/api/formance.com/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func convertUnstructured[T client.Object](v any) T {
	var t T
	t = reflect.New(reflect.TypeOf(t).Elem()).Interface().(T)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		v.(*unstructured.Unstructured).Object, t); err != nil {
		panic(err)
	}
	return t
}

func VersionsEventHandler(logger logging.Logger, reporter MembershipReporter) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {

			version := convertUnstructured[*v1beta1.Versions](obj)

			logger.Infof("Detect versions '%s' added", version.Name)
			if err := reporter.UpsertVersion(
				context.Background(),
				version.Name,
				version.Spec,
				version.Annotations["formance.com/deprecated"] == "true",
			); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {

			oldVersions := convertUnstructured[*v1beta1.Versions](oldObj)
			newVersions := convertUnstructured[*v1beta1.Versions](newObj)

			if reflect.DeepEqual(oldVersions.Spec, newVersions.Spec) {
				return
			}

			logger.Infof("Detect versions '%s' modified", newVersions.Name)
			if err := reporter.UpsertVersion(
				context.Background(),
				newVersions.Name,
				newVersions.Spec,
				newVersions.Annotations["formance.com/deprecated"] == "true",
			); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			version := convertUnstructured[*v1beta1.Versions](obj)

			logger.Infof("Detect versions '%s' as deleted", version.Name)
			if err := reporter.DeleteVersion(context.Background(), version.Name); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
	}
}

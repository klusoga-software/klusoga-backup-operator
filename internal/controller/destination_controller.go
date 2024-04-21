/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"reflect"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/klusoga-software/klusoga-backup-operator/api/types"
	backupv1alpha1 "github.com/klusoga-software/klusoga-backup-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
)

// DestinationReconciler reconciles a Destination object
type DestinationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=backup.klusoga.de,resources=destinations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.klusoga.de,resources=destinations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.klusoga.de,resources=destinations/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Destination object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *DestinationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	destination := &backupv1alpha1.Destination{}
	err := r.Get(ctx, req.NamespacedName, destination)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Destination not found", "Name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Mssql Target", "Name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	foundConfig := &corev1.ConfigMap{}
	if err := r.Get(ctx, k8sTypes.NamespacedName{Name: destination.Name, Namespace: destination.Namespace}, foundConfig); err != nil {
		if errors.IsNotFound(err) {
			config, err := r.createDestinationConfigmap(destination)
			if err != nil {
				logger.Error(err, "Failed to create Configmap", "Name", config.Name)
				return ctrl.Result{}, err
			}

			err = r.Create(ctx, config)
			if err != nil {
				logger.Error(err, "Failed to create Configmap", "Name", config.Name)
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	}

	config, err := r.createDestinationConfigmap(destination)
	if err != nil {
		logger.Error(err, "Failed to create Configmap", "Name", config.Name)
		return ctrl.Result{}, err
	}

	if !reflect.DeepEqual(config.Data, foundConfig.Data) {
		if err = r.Update(ctx, config); err != nil {
			logger.Error(err, "Failed to update Configmap", "Name", config.Name)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DestinationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1alpha1.Destination{}).
		Complete(r)
}

func (r *DestinationReconciler) createDestinationConfigmap(destination *backupv1alpha1.Destination) (*corev1.ConfigMap, error) {
	dest := types.Destination{
		Name: destination.Name,
		Type: destination.Spec.Type,
	}

	switch destination.Spec.Type {
	case types.Aws:
		dest.Bucket = destination.Spec.AwsDestinationSpec.Bucket
		dest.Region = destination.Spec.AwsDestinationSpec.Region
	}

	destinationFile := types.DestinationFile{Destinations: []types.Destination{dest}}
	data, err := yaml.Marshal(destinationFile)
	if err != nil {
		return nil, err
	}

	config := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destination.Name,
			Namespace: destination.Namespace,
		},
		Data: map[string]string{"destinations.yaml": string(data)},
	}

	err = controllerutil.SetControllerReference(destination, config, r.Scheme)
	if err != nil {
		return nil, err
	}

	return config, nil
}

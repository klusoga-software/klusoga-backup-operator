/*
Copyright 2023.

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

package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1alpha1 "github.com/klusoga-software/klusoga-backup-operator/api/v1alpha1"
)

const loadbalancerFinalizer = "server.klusoga.de/cleanup"

// MssqlTargetReconciler reconciles a MssqlTarget object
type MssqlTargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=backup.klusoga.de,resources=mssqltargets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.klusoga.de,resources=mssqltargets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.klusoga.de,resources=mssqltargets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MssqlTarget object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MssqlTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mssqlTarget := &backupv1alpha1.MssqlTarget{}
	err := r.Get(ctx, req.NamespacedName, mssqlTarget)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Mssql Target not found", "Name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Mssql Target", "Name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling Mssql Target", "Name", mssqlTarget.Name)

	depl, err := r.getDeployment(ctx, req, mssqlTarget)

	containers := depl.Spec.Template.Spec.Containers
	found := false
	for _, c := range containers {
		if c.Name == "klusoga-backup" {
			found = true
		}
	}

	if !found {
		if err := r.createBackupContainer(ctx, depl, mssqlTarget); err != nil {
			return ctrl.Result{}, err
		}
	}

	isMemcachedMarkedToBeDeleted := mssqlTarget.GetDeletionTimestamp() != nil
	if isMemcachedMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(mssqlTarget, loadbalancerFinalizer) {
			if err := r.cleanupDeployment(ctx, mssqlTarget); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(mssqlTarget, loadbalancerFinalizer)
			err := r.Update(ctx, mssqlTarget)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(mssqlTarget, loadbalancerFinalizer) {
		controllerutil.AddFinalizer(mssqlTarget, loadbalancerFinalizer)
		err = r.Update(ctx, mssqlTarget)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MssqlTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1alpha1.MssqlTarget{}).
		Owns(&appsv1.Deployment{}).
		Owns(&v1.Secret{}).
		Complete(r)
}

func (r *MssqlTargetReconciler) getDeployment(ctx context.Context, req ctrl.Request, mssqlTarget *backupv1alpha1.MssqlTarget) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: mssqlTarget.Spec.DeploymentName, Namespace: mssqlTarget.Namespace}, deployment)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *MssqlTargetReconciler) createBackupContainer(ctx context.Context, deployment *appsv1.Deployment, mssqlTarget *backupv1alpha1.MssqlTarget) error {
	container := v1.Container{
		Name:    "klusoga-backup",
		Image:   mssqlTarget.Spec.Image,
		Command: []string{"/klusoga-backup-agent"},
		Args:    []string{"backup", "--host", "127.0.0.1", "-p", "gS75e4nt2vr263Vc6fgw", "-u", "sa", "-t", "mssql", "--port", "1433", "--path", "/", "--databases", "master", "--schedule", "*/20 * * * * *"},
	}

	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, container)

	err := r.Client.Update(ctx, deployment)
	if err != nil {
		return err
	}

	return nil
}

func (r *MssqlTargetReconciler) cleanupDeployment(ctx context.Context, mssqlTarget *backupv1alpha1.MssqlTarget) error {
	return nil
}

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

package controller

import (
	"context"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	types2 "github.com/klusoga-software/klusoga-backup-operator/api/types"
	backupv1alpha1 "github.com/klusoga-software/klusoga-backup-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
)

type SqlCredentials struct {
	Username string
	Password string
}

// MssqlTargetReconciler reconciles a MssqlTarget object
type MssqlTargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=klusoga.de,resources=mssqltargets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=klusoga.de,resources=mssqltargets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=klusoga.de,resources=mssqltargets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MssqlTarget object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
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

	foundCronjob := &batchv1.CronJob{}
	err = r.Get(ctx, types.NamespacedName{Name: mssqlTarget.Name, Namespace: mssqlTarget.Namespace}, foundCronjob)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Mssql Target Cronjob not found. Creating...")
			secret, err := r.getSqlSecret(ctx, mssqlTarget)
			if err != nil {
				return ctrl.Result{}, err
			}

			cron, err := r.createCronJob(ctx, mssqlTarget, *secret)
			err = r.Create(ctx, cron)
			if err != nil {
				logger.Error(err, "Error while create cronjob")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	secret, err := r.getSqlSecret(ctx, mssqlTarget)
	if err != nil {
		return ctrl.Result{}, err
	}
	cron, err := r.createCronJob(ctx, mssqlTarget, *secret)
	if !reflect.DeepEqual(cron.Spec, foundCronjob.Spec) {
		if err = r.Update(ctx, cron); err != nil {
			logger.Error(err, "Failed to update Cronjob", "Name", cron.Name)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MssqlTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1alpha1.MssqlTarget{}).
		Complete(r)
}

func (r *MssqlTargetReconciler) getSqlSecret(ctx context.Context, mssqlTarget *backupv1alpha1.MssqlTarget) (*SqlCredentials, error) {
	secret := &v1.Secret{}

	if err := r.Get(ctx, types.NamespacedName{Name: mssqlTarget.Spec.CredentialsRef, Namespace: mssqlTarget.Namespace}, secret); err != nil {
		return nil, err
	}

	sclCredentials := &SqlCredentials{
		Username: string(secret.Data["username"]),
		Password: string(secret.Data["password"]),
	}

	return sclCredentials, nil
}

func (r *MssqlTargetReconciler) getDestinationData(ctx context.Context, mssqlTarget *backupv1alpha1.MssqlTarget) (*backupv1alpha1.Destination, error) {
	destination := &backupv1alpha1.Destination{}

	if err := r.Get(ctx, types.NamespacedName{Name: mssqlTarget.Spec.DestinationRef, Namespace: mssqlTarget.Namespace}, destination); err != nil {
		return nil, err
	}

	return destination, nil
}

func (r *MssqlTargetReconciler) createCronJob(ctx context.Context, mssqlTarget *backupv1alpha1.MssqlTarget, sqlCredentials SqlCredentials) (*batchv1.CronJob, error) {

	destination, err := r.getDestinationData(ctx, mssqlTarget)
	if err != nil {
		return nil, err
	}

	container := v1.Container{
		Name:    "klusoga-backup",
		Image:   mssqlTarget.Spec.Image,
		Env:     r.buildBackupEnvVariables(destination),
		Command: []string{"/klusoga-backup-agent"},
		VolumeMounts: []v1.VolumeMount{
			v1.VolumeMount{
				Name:      "destinations",
				MountPath: "/destinations.yaml",
				SubPath:   "destinations.yaml",
			},
			v1.VolumeMount{
				Name:      "backup",
				MountPath: mssqlTarget.Spec.Path,
			},
		},
		Args: []string{"backup", "--host", mssqlTarget.Spec.Host, "-p", sqlCredentials.Password, "-u", sqlCredentials.Username, "-t", "mssql", "--port", mssqlTarget.Spec.Port, "--path", mssqlTarget.Spec.Path, "--databases", mssqlTarget.Spec.Databases, "--destination", mssqlTarget.Spec.DestinationRef},
	}

	var ttl int32 = 2
	var backoffLimit int32 = 5
	var parallel int32 = 1
	var failedJobCount int32 = 1

	cronjob := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mssqlTarget.Name,
			Namespace: mssqlTarget.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:               mssqlTarget.Spec.Schedule,
			FailedJobsHistoryLimit: &failedJobCount,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &ttl,
					BackoffLimit:            &backoffLimit,
					Parallelism:             &parallel,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers:    []v1.Container{container},
							RestartPolicy: v1.RestartPolicyOnFailure,
							Volumes: []v1.Volume{
								v1.Volume{
									Name: "destinations",
									VolumeSource: v1.VolumeSource{
										ConfigMap: &v1.ConfigMapVolumeSource{
											Items: []v1.KeyToPath{v1.KeyToPath{
												Key:  "destinations.yaml",
												Path: "destinations.yaml",
											}},
											LocalObjectReference: v1.LocalObjectReference{
												Name: destination.Name,
											},
										},
									},
								},
								v1.Volume{
									Name: "backup",
									VolumeSource: v1.VolumeSource{
										PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
											ClaimName: mssqlTarget.Spec.PersistentVolumeClaimName,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err = controllerutil.SetControllerReference(mssqlTarget, &cronjob, r.Scheme)
	if err != nil {
		return nil, err
	}

	return &cronjob, nil
}

func (r *MssqlTargetReconciler) buildBackupEnvVariables(destination *backupv1alpha1.Destination) []v1.EnvVar {
	var envVars []v1.EnvVar

	switch destination.Spec.Type {
	case types2.Aws:
		envVars = append(
			envVars, v1.EnvVar{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						Key: "AWS_ACCESS_KEY_ID",
						LocalObjectReference: v1.LocalObjectReference{
							Name: destination.Spec.AwsDestinationSpec.SecretRef,
						},
					},
				},
			},
		)

		envVars = append(
			envVars, v1.EnvVar{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						Key: "AWS_SECRET_ACCESS_KEY",
						LocalObjectReference: v1.LocalObjectReference{
							Name: destination.Spec.AwsDestinationSpec.SecretRef,
						},
					},
				},
			},
		)
	}

	envVars = append(envVars, v1.EnvVar{
		Name:  "DESTINATION_FILE_PATH",
		Value: "/destinations.yaml",
	})

	return envVars
}

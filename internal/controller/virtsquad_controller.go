/*
Copyright 2025.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "github.com/mshort55/virtsquad-operator/api/v1"
)

// VirtSquadReconciler reconciles a VirtSquad object
type VirtSquadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.mshort55.io,resources=virtsquads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.mshort55.io,resources=virtsquads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.mshort55.io,resources=virtsquads/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

const (
	virtSquadFinalizer = "virtsquad.mshort55.io/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *VirtSquadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the VirtSquad instance
	virtSquad := &appsv1.VirtSquad{}
	err := r.Get(ctx, req.NamespacedName, virtSquad)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("VirtSquad resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get VirtSquad")
		return ctrl.Result{}, err
	}

	// Check if the VirtSquad instance is marked to be deleted
	if virtSquad.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(virtSquad, virtSquadFinalizer) {
			// Run finalization logic for virtSquadFinalizer
			if err := r.finalizeVirtSquad(ctx, virtSquad); err != nil {
				return ctrl.Result{}, err
			}

			// Remove virtSquadFinalizer
			controllerutil.RemoveFinalizer(virtSquad, virtSquadFinalizer)
			err := r.Update(ctx, virtSquad)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(virtSquad, virtSquadFinalizer) {
		controllerutil.AddFinalizer(virtSquad, virtSquadFinalizer)
		err = r.Update(ctx, virtSquad)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile each team member
	status := &appsv1.VirtSquadStatus{}

	if err := r.reconcileTeamMember(ctx, virtSquad, "oksana", virtSquad.Spec.Oksana, &status.OksanaPods); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileTeamMember(ctx, virtSquad, "kurtis", virtSquad.Spec.Kurtis, &status.KurtisPods); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileTeamMember(ctx, virtSquad, "matt", virtSquad.Spec.Matt, &status.MattPods); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileTeamMember(ctx, virtSquad, "kike", virtSquad.Spec.Kike, &status.KikePods); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	status.TotalPods = int32(len(status.OksanaPods) + len(status.KurtisPods) + len(status.MattPods) + len(status.KikePods))

	// Count ready pods
	readyCount, err := r.countReadyPods(ctx, virtSquad)
	if err != nil {
		log.Error(err, "Failed to count ready pods")
	} else {
		status.ReadyPods = readyCount
	}

	virtSquad.Status = *status
	err = r.Status().Update(ctx, virtSquad)
	if err != nil {
		log.Error(err, "Failed to update VirtSquad status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileTeamMember handles pod reconciliation for a single team member
func (r *VirtSquadReconciler) reconcileTeamMember(ctx context.Context, virtSquad *appsv1.VirtSquad, memberName string, memberSpec *appsv1.TeamMemberSpec, statusPods *[]string) error {
	log := logf.FromContext(ctx)

	if memberSpec == nil || memberSpec.Name == nil {
		// Team member not specified, delete any existing pods
		return r.deleteTeamMemberPods(ctx, virtSquad, memberName, statusPods)
	}

	// Determine desired replica count
	desiredReplicas := int32(1)
	if memberSpec.Replicas != nil {
		desiredReplicas = *memberSpec.Replicas
	}

	// Get existing pods for this team member
	existingPods := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(virtSquad.Namespace),
		client.MatchingLabels{
			"app":                          "virtsquad",
			"virtsquad.mshort55.io/member": memberName,
			"virtsquad.mshort55.io/squad":  virtSquad.Name,
		},
	}

	if err := r.List(ctx, existingPods, listOpts...); err != nil {
		log.Error(err, "Failed to list existing pods", "member", memberName)
		return err
	}

	currentReplicas := int32(len(existingPods.Items))

	// Scale up if needed
	if currentReplicas < desiredReplicas {
		for i := currentReplicas; i < desiredReplicas; i++ {
			if err := r.createPodForMember(ctx, virtSquad, memberName, *memberSpec.Name, i); err != nil {
				return err
			}
		}
	}

	// Scale down if needed
	if currentReplicas > desiredReplicas {
		podsToDelete := currentReplicas - desiredReplicas
		for i := int32(0); i < podsToDelete && i < int32(len(existingPods.Items)); i++ {
			if err := r.Delete(ctx, &existingPods.Items[i]); err != nil {
				log.Error(err, "Failed to delete pod", "pod", existingPods.Items[i].Name)
				return err
			}
		}
	}

	// Update status with current pod names
	*statusPods = make([]string, 0, len(existingPods.Items))
	for _, pod := range existingPods.Items {
		*statusPods = append(*statusPods, pod.Name)
	}

	return nil
}

// createPodForMember creates a new pod for a team member
func (r *VirtSquadReconciler) createPodForMember(ctx context.Context, virtSquad *appsv1.VirtSquad, memberName, podBaseName string, replica int32) error {
	log := logf.FromContext(ctx)

	podName := fmt.Sprintf("%s-%d", podBaseName, replica)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: virtSquad.Namespace,
			Labels: map[string]string{
				"app":                          "virtsquad",
				"virtsquad.mshort55.io/member": memberName,
				"virtsquad.mshort55.io/squad":  virtSquad.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  memberName,
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							Name:          "http",
						},
					},
				},
			},
		},
	}

	// Set VirtSquad instance as the owner and controller
	if err := controllerutil.SetControllerReference(virtSquad, pod, r.Scheme); err != nil {
		return err
	}

	log.Info("Creating pod", "pod", podName, "member", memberName)
	return r.Create(ctx, pod)
}

// deleteTeamMemberPods deletes all pods for a team member
func (r *VirtSquadReconciler) deleteTeamMemberPods(ctx context.Context, virtSquad *appsv1.VirtSquad, memberName string, statusPods *[]string) error {
	log := logf.FromContext(ctx)

	// Get existing pods for this team member
	existingPods := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(virtSquad.Namespace),
		client.MatchingLabels{
			"app":                          "virtsquad",
			"virtsquad.mshort55.io/member": memberName,
			"virtsquad.mshort55.io/squad":  virtSquad.Name,
		},
	}

	if err := r.List(ctx, existingPods, listOpts...); err != nil {
		log.Error(err, "Failed to list existing pods for deletion", "member", memberName)
		return err
	}

	// Delete all pods for this member
	for _, pod := range existingPods.Items {
		if err := r.Delete(ctx, &pod); err != nil {
			log.Error(err, "Failed to delete pod", "pod", pod.Name)
			return err
		}
	}

	// Clear the status
	*statusPods = []string{}

	return nil
}

// countReadyPods counts the number of ready pods managed by this VirtSquad
func (r *VirtSquadReconciler) countReadyPods(ctx context.Context, virtSquad *appsv1.VirtSquad) (int32, error) {
	pods := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(virtSquad.Namespace),
		client.MatchingLabels{
			"app":                         "virtsquad",
			"virtsquad.mshort55.io/squad": virtSquad.Name,
		},
	}

	if err := r.List(ctx, pods, listOpts...); err != nil {
		return 0, err
	}

	readyCount := int32(0)
	for _, pod := range pods.Items {
		if isPodReady(&pod) {
			readyCount++
		}
	}

	return readyCount, nil
}

// isPodReady checks if a pod is ready
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// finalizeVirtSquad handles cleanup when a VirtSquad is deleted
func (r *VirtSquadReconciler) finalizeVirtSquad(ctx context.Context, virtSquad *appsv1.VirtSquad) error {
	log := logf.FromContext(ctx)

	// Delete all pods managed by this VirtSquad
	pods := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(virtSquad.Namespace),
		client.MatchingLabels{
			"app":                         "virtsquad",
			"virtsquad.mshort55.io/squad": virtSquad.Name,
		},
	}

	if err := r.List(ctx, pods, listOpts...); err != nil {
		log.Error(err, "Failed to list pods for cleanup")
		return err
	}

	for _, pod := range pods.Items {
		if err := r.Delete(ctx, &pod); err != nil {
			log.Error(err, "Failed to delete pod during cleanup", "pod", pod.Name)
			return err
		}
	}

	log.Info("Successfully finalized VirtSquad", "virtsquad", virtSquad.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtSquadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.VirtSquad{}).
		Owns(&corev1.Pod{}).
		Named("virtsquad").
		Complete(r)
}

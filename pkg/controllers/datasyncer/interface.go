package datasyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/kubeflag/kubeflag/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SyncableObject interface {
	ctrlruntimeclient.Object
	GetData() map[string][]byte
	SetData(map[string][]byte)
	GetTypeMeta() metav1.TypeMeta
	GetBaseObject() ctrlruntimeclient.Object
}

type SecretWrapper struct {
	*corev1.Secret
}

func (s SecretWrapper) GetData() map[string][]byte {
	return s.Data
}
func (s SecretWrapper) SetData(d map[string][]byte) {
	s.Data = d
}
func (s SecretWrapper) GetTypeMeta() metav1.TypeMeta {
	return s.TypeMeta
}

type ConfigMapWrapper struct {
	*corev1.ConfigMap
}

func (s SecretWrapper) GetBaseObject() ctrlruntimeclient.Object    { return s.Secret }
func (c ConfigMapWrapper) GetBaseObject() ctrlruntimeclient.Object { return c.ConfigMap }

func (c ConfigMapWrapper) GetData() map[string][]byte {
	// Convert string -> []byte for unified comparison
	data := make(map[string][]byte, len(c.Data))
	for k, v := range c.Data {
		data[k] = []byte(v)
	}
	return data
}
func (c ConfigMapWrapper) SetData(d map[string][]byte) {
	// Convert []byte -> string
	cmData := make(map[string]string, len(d))
	for k, v := range d {
		cmData[k] = string(v)
	}
	c.Data = cmData
}
func (c ConfigMapWrapper) GetTypeMeta() metav1.TypeMeta {
	return c.TypeMeta
}

func (r *DataSyncerReconciler) reconcileDataObject(ctx context.Context, obj SyncableObject) (reconcile.Result, error) {
	if isSource(obj) {
		challengesNames, err := getChallengeNamesFromAnnotation(obj)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to parse annotations: %w", err)
		}

		if err = r.cleanupUndesiredCopies(ctx, obj, challengesNames); err != nil {
			return reconcile.Result{}, err
		}

		if obj.GetDeletionTimestamp() != nil && kubernetes.HasFinalizer(obj, CleanupFinalizer) {
			if len(challengesNames) > 0 {
				if err = r.cleanupCopies(ctx, obj, challengesNames); err != nil {
					return reconcile.Result{}, err
				}
			}

			kubernetes.RemoveFinalizer(obj, CleanupFinalizer)
			if err = r.Update(ctx, obj); err != nil {
				return reconcile.Result{}, err
			}
		}

		for _, challengeName := range challengesNames {
			if err = r.syncToNamespace(ctx, obj, challengeName); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to sync secret to namespace %s: %w", challengeName, err)
			}
			if !kubernetes.HasFinalizer(obj, CleanupFinalizer) {
				kubernetes.AddFinalizer(obj, CleanupFinalizer)
				if err = r.Update(ctx, obj); err != nil {
					return reconcile.Result{}, err
				}
			}
		}

	} else {
		var source ctrlruntimeclient.Object
		switch obj.(type) {
		case SecretWrapper:
			source = &corev1.Secret{}
		}
		namespaced := getSource(obj)
		if namespaced == nil {
			return reconcile.Result{}, fmt.Errorf("secret don't have any source obeject")
		}
		fmt.Println(namespaced.Name, namespaced.Namespace)
		if err := r.Get(ctx, *namespaced, source); err != nil {
			return reconcile.Result{}, err
		}

		// If object is deleted now, reconcile the source to create another one if it's necessary.
		if obj.GetDeletionTimestamp() != nil && kubernetes.HasAnyFinalizer(obj, SyncFinalizer) {
			annotations := source.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["datasyncer.kubeflag.io/last-trigger"] = time.Now().Format(time.RFC3339)

			source.SetAnnotations(annotations)
			if err := r.Update(ctx, source); err != nil {
				return reconcile.Result{}, err
			}
			kubernetes.RemoveFinalizer(obj, SyncFinalizer)
			if err := r.Update(ctx, obj.GetBaseObject()); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to remove finalizer in object %w", err)
			}
			return reconcile.Result{}, nil
		}
		var syncable SyncableObject
		switch s := source.(type) {
		case *corev1.Secret:
			syncable = &SecretWrapper{Secret: s}
		case *corev1.ConfigMap:
			syncable = &ConfigMapWrapper{ConfigMap: s}
		default:
			return reconcile.Result{}, fmt.Errorf("unsupported source type: %T", source)
		}

		if !equality.Semantic.DeepEqual(syncable.GetData(), obj.GetData()) {
			obj.SetData(syncable.GetData())
			return reconcile.Result{}, r.Update(ctx, obj)
		}
	}

	return reconcile.Result{}, nil
}

func (r *DataSyncerReconciler) syncToNamespace(ctx context.Context, source SyncableObject, targetNamespace string) error {
	var newObj ctrlruntimeclient.Object
	switch source.(type) {
	case SecretWrapper:
		newObj = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      source.GetName(),
				Namespace: targetNamespace,
				Labels: map[string]string{
					ManagedLabel: "true",
					SourceLabel:  fmt.Sprintf("%s---%s", source.GetNamespace(), source.GetName()),
				},
				Finalizers: []string{SyncFinalizer},
			},
			Data: source.GetData(),
		}
	case ConfigMapWrapper:
		newObj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      source.GetName(),
				Namespace: targetNamespace,
				Labels: map[string]string{
					ManagedLabel: "true",
					SourceLabel:  fmt.Sprintf("%s---%s", source.GetNamespace(), source.GetName()),
				},
				Finalizers: []string{SyncFinalizer},
			},
			BinaryData: source.GetData(),
		}
	}

	return r.createOrUpdate(ctx, newObj)
}

func (r *DataSyncerReconciler) cleanupCopies(ctx context.Context, source SyncableObject, challenges []string) error {
	// Build a set for quick membership tests
	desired := sets.NewString(challenges...)

	for _, ns := range desired.List() {
		var copy ctrlruntimeclient.Object
		switch source.(type) {
		case SecretWrapper:
			copy = &corev1.Secret{}
		}
		key := types.NamespacedName{Name: source.GetGenerateName(), Namespace: ns}

		err := r.Get(ctx, key, copy)
		if apierrors.IsNotFound(err) {
			// nothing to delete in this namespace
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get %s %s/%s: %w", source.GetTypeMeta().Kind, ns, source.GetName(), err)
		}

		// Optional safety check: ensure it is a managed copy
		if copy.GetLabels() != nil && copy.GetLabels()[ManagedLabel] != "true" {
			continue
		}

		if err := r.Delete(ctx, copy); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", source.GetTypeMeta().Kind, ns, source.GetName(), err)
		}
	}
	return nil
}

func (r *DataSyncerReconciler) cleanupUndesiredCopies(ctx context.Context, source SyncableObject, desiredNamespaces []string) error {
	// Fast membership check
	desired := sets.NewString(desiredNamespaces...)

	selector := ctrlruntimeclient.MatchingLabels{
		ManagedLabel: "true",
		SourceLabel:  fmt.Sprintf("%s---%s", source.GetNamespace(), source.GetName()),
	}

	var list ctrlruntimeclient.ObjectList
	switch source.(type) {
	case SecretWrapper:
		list = &corev1.SecretList{}
	}

	if err := r.List(ctx, list, selector); err != nil {
		return fmt.Errorf("list managed copies: %w", err)
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		return fmt.Errorf("extract list items: %w", err)
	}

	for _, s := range items {
		// ✨ Convert to client.Object (has GetName()/GetNamespace())
		obj, ok := s.(ctrlruntimeclient.Object)
		if !ok {
			return fmt.Errorf("failt to convert runtime.Object to ctrlruntimeclient.Object")
		}
		if desired.Has(obj.GetNamespace()) {
			continue // still wanted
		}

		// Delete copies in namespaces no longer desired
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete stale copy %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}

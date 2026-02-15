package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeStore struct {
	client dynamic.Interface
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewKubeStore() (*KubeStore, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, err
		}
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &KubeStore{client: client}, nil
}

func (k *KubeStore) UpsertNodeDisk(ctx context.Context, nd NodeDiskUpsert) error {
	resource := k.client.Resource(shared.GVR)

	return retryOnConflict(func() error {
		existing, err := resource.Get(ctx, nd.Name, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": fmt.Sprintf("%s/%s", shared.Group, shared.Version),
				"kind":       shared.Kind,
				"metadata": map[string]any{
					"name":   nd.Name,
					"labels": nd.Labels,
				},
				"spec": map[string]any{
					"nodeName": nd.NodeName,
					"diskID":   nd.DiskID,
					"path":     nd.Path,
					"desired": map[string]any{
						"state": nd.DefaultTo.State,
						"owner": nd.DefaultTo.Owner,
					},
				},
			}}
			_, createErr := resource.Create(ctx, obj, metav1.CreateOptions{})
			return createErr
		}

		modified := false
		labels := existing.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for key, value := range nd.Labels {
			if labels[key] != value {
				labels[key] = value
				modified = true
			}
		}
		if modified {
			existing.SetLabels(labels)
		}

		spec, _, _ := unstructured.NestedMap(existing.Object, "spec")
		if spec == nil {
			spec = map[string]any{}
			modified = true
		}
		if getString(spec, "nodeName") == "" {
			spec["nodeName"] = nd.NodeName
			modified = true
		}
		if getString(spec, "diskID") == "" {
			spec["diskID"] = nd.DiskID
			modified = true
		}
		if getString(spec, "path") == "" {
			spec["path"] = nd.Path
			modified = true
		}

		desired, _, _ := unstructured.NestedMap(spec, "desired")
		if desired == nil {
			desired = map[string]any{}
			modified = true
		}
		if getString(desired, "state") == "" {
			desired["state"] = string(nd.DefaultTo.State)
			modified = true
		}
		if getString(desired, "owner") == "" && nd.DefaultTo.Owner != "" {
			desired["owner"] = nd.DefaultTo.Owner
			modified = true
		}
		spec["desired"] = desired
		existing.Object["spec"] = spec

		if !modified {
			return nil
		}
		_, err = resource.Update(ctx, existing, metav1.UpdateOptions{})
		return err
	})
}

func (k *KubeStore) PatchNodeStatus(ctx context.Context, name string, st NodeStatusPatch) error {
	resource := k.client.Resource(shared.GVR)
	nodeStatus := map[string]any{
		"phase":        st.Phase,
		"lastSeenTime": metav1.NewTime(st.LastSeenTime),
		"message":      st.Message,
	}
	if st.CapacityBytes != nil {
		nodeStatus["capacityBytes"] = *st.CapacityBytes
	}
	statusPayload := map[string]any{
		"status": map[string]any{
			"node": nodeStatus,
		},
	}
	raw, err := json.Marshal(statusPayload)
	if err != nil {
		return err
	}
	_, err = resource.Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{}, "status")
	return err
}

func (k *KubeStore) WatchNodeDisks(ctx context.Context, nodeName string) (<-chan NodeDiskEvent, error) {
	out := make(chan NodeDiskEvent, 128)
	resource := k.client.Resource(shared.GVR)
	selector := fmt.Sprintf("%s=%s", shared.LabelNode, nodeName)

	go func() {
		defer close(out)
		var resourceVersion string

		for {
			if err := ctx.Err(); err != nil {
				return
			}

			list, err := resource.List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				time.Sleep(reconnectDelay(2 * time.Second))
				continue
			}
			resourceVersion = list.GetResourceVersion()
			for i := range list.Items {
				event, ok := objectToEvent(&list.Items[i], NodeDiskEventSync)
				if !ok {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- event:
				}
			}

			watcher, err := resource.Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: resourceVersion})
			if err != nil {
				time.Sleep(reconnectDelay(2 * time.Second))
				continue
			}

		watchLoop:
			for {
				select {
				case <-ctx.Done():
					watcher.Stop()
					return
				case evt, ok := <-watcher.ResultChan():
					if !ok {
						break watchLoop
					}
					obj, ok := evt.Object.(*unstructured.Unstructured)
					if !ok {
						continue
					}
					resourceVersion = obj.GetResourceVersion()
					event, ok := objectToEvent(obj, toNodeDiskEventType(evt.Type))
					if !ok {
						continue
					}
					select {
					case <-ctx.Done():
						watcher.Stop()
						return
					case out <- event:
					}
				}
			}
			watcher.Stop()
			time.Sleep(reconnectDelay(1 * time.Second))
		}
	}()

	return out, nil
}

func objectToEvent(obj *unstructured.Unstructured, eventType NodeDiskEventType) (NodeDiskEvent, bool) {
	nodeName, _, _ := unstructured.NestedString(obj.Object, "spec", "nodeName")
	diskID, _, _ := unstructured.NestedString(obj.Object, "spec", "diskID")
	path, _, _ := unstructured.NestedString(obj.Object, "spec", "path")
	desiredStateRaw, _, _ := unstructured.NestedString(obj.Object, "spec", "desired", "state")
	desiredOwner, _, _ := unstructured.NestedString(obj.Object, "spec", "desired", "owner")
	if nodeName == "" || diskID == "" {
		return NodeDiskEvent{}, false
	}
	desiredState := v1alpha1.DesiredStateAvailable
	if desiredStateRaw == string(v1alpha1.DesiredStateReserved) {
		desiredState = v1alpha1.DesiredStateReserved
	}
	return NodeDiskEvent{
		Type:         eventType,
		Name:         obj.GetName(),
		NodeName:     nodeName,
		DiskID:       diskID,
		Path:         path,
		DesiredState: desiredState,
		DesiredOwner: desiredOwner,
	}, true
}

func toNodeDiskEventType(t watch.EventType) NodeDiskEventType {
	switch t {
	case watch.Added:
		return NodeDiskEventAdded
	case watch.Modified:
		return NodeDiskEventModified
	case watch.Deleted:
		return NodeDiskEventDeleted
	default:
		return NodeDiskEventModified
	}
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func retryOnConflict(fn func() error) error {
	return wait.ExponentialBackoff(wait.Backoff{Duration: 100 * time.Millisecond, Factor: 1.5, Steps: 5}, func() (bool, error) {
		err := fn()
		if err == nil {
			return true, nil
		}
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return false, err
	})
}

func reconnectDelay(base time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	// Add small jitter so agents do not reconnect in lockstep.
	return base + time.Duration(rand.Intn(500))*time.Millisecond
}

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"context"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Test mergeServicePorts
func TestMergeServicePorts(t *testing.T) {
	svcSpec := &corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(443)},
		},
	}

	requiredPorts := []corev1.ServicePort{
		{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8080)},
		{Name: "metrics", Port: 9090, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(9090)},
	}

	expectedPorts := []corev1.ServicePort{
		{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8080)},
		{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(443)},
		{Name: "metrics", Port: 9090, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(9090)},
	}

	mergeServicePorts(svcSpec, requiredPorts)

	sort.Slice(svcSpec.Ports, func(i, j int) bool {
		return svcSpec.Ports[i].Name < svcSpec.Ports[j].Name
	})

	assert.Equal(t, 3, len(svcSpec.Ports), "Expected 3 ports")
	assert.Equal(t, expectedPorts, svcSpec.Ports)
}

// Test deDupAndSortInLocalNS
func TestDeDupAndSortInLocalNS(t *testing.T) {
	var podCRMap = CRMap[corev1.Pod, *corev1.Pod, *corev1.PodList]{
		NewList:  func() *corev1.PodList { return new(corev1.PodList) },
		ItemsFor: func(list *corev1.PodList) []corev1.Pod { return list.Items },
		AsObject: func(item *corev1.Pod) *corev1.Pod { return item },
	}

	mockList := &corev1.PodList{
		Items: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-b"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-a"}}, // Duplicate
		},
	}

	sorted := deDupAndSortInLocalNS(podCRMap, mockList)

	expectedNames := []string{"pod-a", "pod-b"}
	actualNames := make([]string, len(sorted))
	for i, pod := range sorted {
		actualNames[i] = pod.GetName()
	}

	assert.Equal(t, 2, len(sorted), "Expected 2 pods")
	assert.Equal(t, expectedNames, actualNames, "Expected pod names to match")
}

// Test setCondition
func TestSetCondition(t *testing.T) {
	var conditions []metav1.Condition

	setCondition(&conditions, corev1alpha1.ConditionType("Ready"), metav1.ConditionTrue, corev1alpha1.ConditionReason("DeploymentSucceeded"), "Deployment successful")

	require.Equal(t, 1, len(conditions), "Expected 1 condition")

	condition := conditions[0]
	assert.False(t, condition.Type != "Ready" || condition.Status != metav1.ConditionTrue || condition.Reason != "DeploymentSucceeded", "Condition type should be Ready")
}

// Mock client for reconcileAll
type mockClient struct{}

func (m *mockClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	podList := obj.(*corev1.PodList)
	*podList = corev1.PodList{
		Items: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "test-ns"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "test-ns"}},
		},
	}
	return nil
}

// MockRateLimitingQueue is a simple implementation of the workqueue.RateLimitingInterface for testing purposes.
type MockRateLimitingQueue struct {
	items []reconcile.Request
}

func (q *MockRateLimitingQueue) Add(item interface{}) {
	req, ok := item.(reconcile.Request)
	if ok {
		q.items = append(q.items, req)
	}
}
func (q *MockRateLimitingQueue) Len() int {
	return len(q.items)
}
func (q *MockRateLimitingQueue) Get() (item interface{}, shutdown bool) {
	if len(q.items) == 0 {
		return nil, true
	}
	item, q.items = q.items[0], q.items[1:]
	return item, false
}
func (q *MockRateLimitingQueue) Done(item interface{})                             {}
func (q *MockRateLimitingQueue) ShutDown()                                         {}
func (q *MockRateLimitingQueue) ShuttingDown() bool                                { return false }
func (q *MockRateLimitingQueue) ShutDownWithDrain()                                {}
func (q *MockRateLimitingQueue) AddRateLimited(item interface{})                   {}
func (q *MockRateLimitingQueue) Forget(item interface{})                           {}
func (q *MockRateLimitingQueue) NumRequeues(item interface{}) int                  { return 0 }
func (q *MockRateLimitingQueue) AddAfter(item interface{}, duration time.Duration) {}

func TestReconcileAll(t *testing.T) {
	// Define CRMap
	var podCRMap = CRMap[corev1.Pod, *corev1.Pod, *corev1.PodList]{
		NewList: func() *corev1.PodList {
			return &corev1.PodList{}
		},
		ItemsFor: func(list *corev1.PodList) []corev1.Pod {
			return list.Items
		},
		AsObject: func(item *corev1.Pod) *corev1.Pod {
			return item
		},
	}

	// Scheme setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Fake client setup
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "test-ns"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "test-ns"}},
		).
		Build()

	// Mock queue to capture reconcile requests
	mockQueue := &MockRateLimitingQueue{}

	// Create the handler
	handler := reconcileAll(podCRMap, client)

	// Simulate a create event for a new pod
	p := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-3", Namespace: "test-ns"}}
	e := event.CreateEvent{Object: &p}
	handler.Create(context.TODO(), e, mockQueue)

	// Verify the captured requests
	expectedRequests := sets.NewString(
		"pod-1/test-ns",
		"pod-2/test-ns",
	)

	actualRequests := sets.NewString()
	for _, req := range mockQueue.items {
		actualRequests.Insert(req.Name + "/" + req.Namespace)
	}

	assert.Equal(t, expectedRequests, actualRequests, "Expected reconcile requests to match")
}

func TestMapToStruct(t *testing.T) {

	type example struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	tests := []struct {
		name    string
		data    map[string][]byte
		result  interface{}
		want    interface{}
		wantErr bool
	}{
		{
			name: "Valid mapping to example",
			data: map[string][]byte{
				"username": []byte("testuser"),
				"password": []byte("testpass"),
			},
			result: &example{},
			want: &example{
				Username: "testuser",
				Password: "testpass",
			},
			wantErr: false,
		},
		{
			name: "Missing key in map",
			data: map[string][]byte{
				"username": []byte("testuser"),
			},
			result: &example{},
			want: &example{
				Username: "testuser",
				Password: "",
			},
			wantErr: false,
		},
		{
			name:    "Result is not a pointer",
			data:    map[string][]byte{},
			result:  example{},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Result is nil pointer",
			data:    map[string][]byte{},
			result:  (*example)(nil),
			want:    nil,
			wantErr: true,
		},
		{
			name: "Unsupported field type",
			data: map[string][]byte{"unsupported": []byte("value")},
			result: &struct {
				UnsupportedField int `json:"unsupported"`
			}{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapToStruct(tt.data, tt.result)

			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Did not expect an error but got one")
			}

			if !tt.wantErr {
				assert.Equal(t, tt.want, tt.result, "Result mismatch")
			}
		})
	}
}

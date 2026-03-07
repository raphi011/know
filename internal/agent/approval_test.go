package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalRegistry_RegisterAndResolve(t *testing.T) {
	reg := newApprovalRegistry()
	ch := reg.register("call-1")

	go func() {
		err := reg.resolve(ApprovalResponse{
			CallID: "call-1",
			Action: ApprovalApproveAll,
		})
		require.NoError(t, err)
	}()

	select {
	case resp := <-ch:
		assert.Equal(t, ApprovalApproveAll, resp.Action)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval")
	}
}

func TestApprovalRegistry_ResolveUnknown(t *testing.T) {
	reg := newApprovalRegistry()
	err := reg.resolve(ApprovalResponse{CallID: "nope"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pending approval")
}

func TestApprovalRegistry_Cancel(t *testing.T) {
	reg := newApprovalRegistry()
	ch := reg.register("call-1")
	reg.cancel()
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after cancel")
}

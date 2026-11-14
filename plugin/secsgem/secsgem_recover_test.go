package secsgem

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dashfactory/go-factory-io/pkg/message/secs2"
	"github.com/dashfactory/go-factory-io/pkg/transport/hsms"
	"github.com/seikaikyo/go-edge-gateway/plugin"
)

// fakeEquipment listens on 127.0.0.1 and answers S1F13 with the given
// S1F14 body bytes.
func fakeEquipment(t *testing.T, ctx context.Context, s1f14 []byte) (*hsms.Session, int) {
	t.Helper()
	cfg := hsms.DefaultConfig("127.0.0.1:0", hsms.RolePassive, 1)
	cfg.LinktestInterval = 0
	cfg.T7 = 0
	sess := hsms.NewSession(cfg, slog.Default())
	if err := sess.Connect(ctx); err != nil {
		t.Fatalf("fake equipment listen: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	go func() {
		for {
			msg, err := sess.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			if msg.Header.Stream == 1 && msg.Header.Function == 13 {
				reply := hsms.NewDataMessage(msg.Header.SessionID, 1, 14, false, msg.Header.SystemID, s1f14)
				sess.SendMessage(ctx, reply)
			}
		}
	}()

	_, portStr, _ := net.SplitHostPort(sess.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return sess, port
}

func normalS1F14(t *testing.T) []byte {
	t.Helper()
	data, err := secs2.Encode(secs2.NewList(secs2.NewBinary([]byte{0x00}), secs2.NewList()))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// nestedS1F14 returns L[2] { B 0, <depth nested lists> }.
func nestedS1F14(depth int) []byte {
	data := []byte{0x01, 0x02, 0x21, 0x01, 0x00}
	for i := 0; i < depth-1; i++ {
		data = append(data, 0x01, 0x01)
	}
	return append(data, 0x01, 0x00)
}

func deviceCfg(names []string, ports []int) map[string]any {
	devices := make([]any, len(names))
	for i := range names {
		devices[i] = map[string]any{
			"name":      names[i],
			"host":      "127.0.0.1",
			"port":      ports[i],
			"device_id": 1,
		}
	}
	return map[string]any{"devices": devices}
}

// TestBadReplyAffectsOnlyThatDevice checks that a device whose S1F14 cannot
// be decoded (nesting above the decoder limit) has its own connection
// closed, while another device keeps its connection and keeps delivering
// data. The nesting used here is shallow: it only needs to exceed the
// decoder limit, not exhaust the stack.
func TestBadReplyAffectsOnlyThatDevice(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	goodSess, goodPort := fakeEquipment(t, ctx, normalS1F14(t))
	badSess, badPort := fakeEquipment(t, ctx, nestedS1F14(200))

	p := &Plugin{}
	uplink := make(chan plugin.Message, 64)
	if err := p.Init(deviceCfg([]string{"good", "bad"}, []int{goodPort, badPort}), uplink); err != nil {
		t.Fatalf("Init: %v", err)
	}
	done := make(chan struct{})
	go func() {
		p.Start(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		p.Stop()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("plugin did not stop")
		}
	}()

	select {
	case <-badSess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("connection to the device with the undecodable reply was left open")
	}

	deadline := time.After(5 * time.Second)
	for got := false; !got; {
		select {
		case m := <-uplink:
			got = m.Device == "good" && m.Topic == "secsgem/rx/S1F14"
		case <-deadline:
			t.Fatal("good device did not deliver its S1F14")
		}
	}

	select {
	case <-goodSess.Done():
		t.Fatal("good device was disconnected by the other device's bad reply")
	default:
	}
}

const devicePanicChildEnv = "EDGE_SECSGEM_PANIC_CHILD"

// TestDevicePanicIsRecovered checks that a panic while handling one device
// is contained to that device's connection. The panic is forced by handing
// the plugin an uplink channel that is already closed, so the first message
// it forwards panics on send.
//
// An unrecovered panic in the device goroutine ends the whole test binary,
// so the scenario runs in a child process and the parent checks its exit.
func TestDevicePanicIsRecovered(t *testing.T) {
	if os.Getenv(devicePanicChildEnv) == "1" {
		runDevicePanicScenario(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDevicePanicIsRecovered$", "-test.v")
	cmd.Env = append(os.Environ(), devicePanicChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed (%v); a device panic must not end the process:\n%s",
			err, tailLines(string(out), 20))
	}
	if !strings.Contains(string(out), "device connection closed, process alive") {
		t.Fatalf("child did not reach the end of the scenario:\n%s", tailLines(string(out), 20))
	}
}

func runDevicePanicScenario(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, port := fakeEquipment(t, ctx, normalS1F14(t))

	closed := make(chan plugin.Message)
	close(closed)

	p := &Plugin{}
	if err := p.Init(deviceCfg([]string{"eq"}, []int{port}), closed); err != nil {
		t.Fatalf("Init: %v", err)
	}
	go p.Start(ctx)

	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("device connection still open after a panic")
	}
	if h := p.Health(); h.OK || !strings.Contains(h.LastError, "panic") {
		t.Fatalf("health after panic: OK=%v LastError=%q, want the panic recorded", h.OK, h.LastError)
	}
	t.Log("device connection closed, process alive")
	cancel()
	p.Stop()
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

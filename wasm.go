package faad2

import (
	"context"
	_ "embed"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed faad2.wasm
var faad2Wasm []byte

type wasmContext struct {
	runtime wazero.Runtime
	module  api.Module

	// Cached function references
	fnVersion  api.Function
	fnCreate   api.Function
	fnDestroy  api.Function
	fnInit     api.Function
	fnDecode   api.Function
	fnGetError api.Function
	fnMalloc   api.Function
	fnFree     api.Function
}

var (
	globalCtx   *wasmContext
	globalOnce  sync.Once
	globalMu    sync.Mutex
	errGlobal   error
	globalReset bool
)

func getWasmContext(ctx context.Context) (*wasmContext, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalReset {
		// Reset the once so we can reinitialize after shutdown
		globalOnce = sync.Once{}
		globalReset = false
	}

	globalOnce.Do(func() {
		globalCtx, errGlobal = initWasmContext(ctx)
	})
	return globalCtx, errGlobal
}

// Shutdown releases the global WASM runtime and all associated resources.
//
// After calling Shutdown:
//   - All existing [Decoder], [M4AReader], and [ADTSReader] instances become invalid
//   - Calling methods on closed instances will return errors or panic
//   - New instances can be created, which will lazily reinitialize the runtime
//
// Shutdown is optional but recommended when the application no longer needs
// AAC decoding, as it frees significant memory used by the WASM runtime.
func Shutdown(ctx context.Context) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalCtx != nil && globalCtx.runtime != nil {
		err := globalCtx.runtime.Close(ctx)
		globalCtx = nil
		globalReset = true
		errGlobal = nil
		return err
	}
	return nil
}

func initWasmContext(ctx context.Context) (*wasmContext, error) {
	rt := wazero.NewRuntime(ctx)

	// Instantiate WASI for fd_close, fd_write, fd_seek
	_, err := wasi_snapshot_preview1.Instantiate(ctx, rt)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	// Provide the env module with emscripten_notify_memory_growth (no-op)
	_, err = rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(_ context.Context, _ uint32) {
			// No-op: called when memory grows, we don't need to do anything
		}).
		Export("emscripten_notify_memory_growth").
		Instantiate(ctx)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	compiled, err := rt.CompileModule(ctx, faad2Wasm)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	module, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	wctx := &wasmContext{
		runtime:    rt,
		module:     module,
		fnVersion:  module.ExportedFunction("faad2_version"),
		fnCreate:   module.ExportedFunction("faad2_decoder_create"),
		fnDestroy:  module.ExportedFunction("faad2_decoder_destroy"),
		fnInit:     module.ExportedFunction("faad2_decoder_init"),
		fnDecode:   module.ExportedFunction("faad2_decoder_decode"),
		fnGetError: module.ExportedFunction("faad2_get_error"),
		fnMalloc:   module.ExportedFunction("malloc"),
		fnFree:     module.ExportedFunction("free"),
	}

	return wctx, nil
}

// malloc allocates memory in the WASM module.
func (w *wasmContext) malloc(ctx context.Context, size uint32) (uint32, error) {
	results, err := w.fnMalloc.Call(ctx, uint64(size))
	if err != nil {
		return 0, err
	}
	ptr := uint32(results[0]) //nolint:gosec // WASM pointers are 32-bit
	if ptr == 0 && size > 0 {
		return 0, ErrOutOfMemory
	}
	return ptr, nil
}

// free releases memory in the WASM module.
func (w *wasmContext) free(ctx context.Context, ptr uint32) {
	if ptr != 0 {
		_, _ = w.fnFree.Call(ctx, uint64(ptr))
	}
}

// write copies data to WASM memory at the given pointer.
func (w *wasmContext) write(ptr uint32, data []byte) bool {
	return w.module.Memory().Write(ptr, data)
}

// read copies data from WASM memory at the given pointer.
func (w *wasmContext) read(ptr, size uint32) ([]byte, bool) {
	return w.module.Memory().Read(ptr, size)
}

# Screen image conversion without per-pixel boxing

A bounded local session soak led to CPU and allocation profiles of the existing viewer path. The fixture capture returned a pre-encoded JPEG, so viewer decoding and conversion were still real production-path work, not repeated synthetic capture encoding. The ordinary CPU profile attributed 4.70 of 5.17 sampled CPU-seconds to `imageToRGBA`. The old generic `At`/`Set` loop boxed decoded pixel colors, creating many short-lived allocations. Cumulative allocation is not retained memory, and the profiles did not establish a leak.

The conversion now uses standard `image/draw` bulk `Src` conversion. It still allocates an independent normalized RGBA buffer for non-RGBA input, and still returns an existing RGBA input directly. No buffer pool, shared-frame lifetime, callback scheduling, dimensions, codec or protocol changes.

## Correctness and focused comparison

Exact output bytes and bounds are compared against an independent copy of the previous converter for decoded JPEG, all six YCbCr subsampling modes, subimages and nonzero bounds, grayscale, alpha, NRGBA, NRGBA64, palette and empty images. Existing RGBA ownership and independent non-RGBA output are checked. A before/after gradient render was inspected with the same colors, diagonal boundary and crop. Twenty focused race repetitions cover conversion, viewer/callback, composite and recorder families; full repository race tests, vet and source checks pass locally.

The bounded-allocation regression fails against original production source with 153,602 allocations for 320x240. Three repeated local conversion microbenchmarks, Linux amd64, AMD EPYC 9655:

| Image size | Previous time | Bulk time | Previous bytes/op | Bulk bytes/op | Previous allocations/op | Bulk allocations/op |
| --- | --- | --- | --- | --- | --- | --- |
| 320x240 | 2.51-2.54ms | 0.689-0.697ms | 925,762 | 311,360 | 153,602 | 2 |
| 1280x720 | 30.84-31.23ms | 8.81-8.89ms | 11,059,280-11,059,282 | 3,686,464 | 1,843,202 | 2 |

[Raw benchmark output](benchmarks/image-conversion-2026-11-20.txt). These are conversion-only measurements, not actual display latency, VPN throughput or fleet capacity.

## Same-workload profile check

Two concurrent real loopback TCP host/viewer sessions ran for 90 seconds with synthetic 10fps capture, bidirectional chat, synthetic input and repeated 64KiB file sends. Each run completed 1,800 frames,1,440 chat callbacks and720 verified file/ACK transfers. The ordinary before profile was compared with two ordinary after runs; one race-enabled before/after pair was measured separately.

| Build | Cumulative Go allocation before | After | Sampled CPU time before | After |
| --- | --- | --- | --- | --- |
| Ordinary | 2.621GB | 1.515GB in both runs | 5.17s | 1.30s and 1.60s |
| Race-enabled | 6.006GB | 1.580GB | 46.17s | 18.06s |

The race detector materially changes workload cost. Profiles are short samples with scheduling variance, not a fixed performance guarantee or peak RSS measurement. Output buffers, JPEG decoding, file copying and queue-owned copies still allocate. All measured runs completed and returned queue/table counts to zero after explicit teardown; no retained-memory leak or hours-long stability claim follows. Synthetic input assertions checked progress per injector, not exact delivery of every sent event. The temporary soak harness and profiles were retained locally, not added as production behavior or a new CI soak.

Fresh normal hosted verification for this source is pending. The normal native test selector adds conversion checks without changing workflow triggers or settings. No Multies work, native/personal-device acceptance, file-entry/reception enablement, production service/configuration, routing/default change or app release is implied.

```sh
go test -race -count=20 -timeout=2m ./internal/session -run '^TestImageToRGBA|^TestFrameCallback|^TestViewer|^TestComposite|^TestRecorder'
go test ./internal/session -run '^$' -bench '^BenchmarkImageConversion$' -benchmem -benchtime=200ms -count=3
go test -race -count=1 -timeout=5m ./...
go vet ./...
```

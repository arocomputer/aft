# Benchmarks

Run everything and write a timestamped report here:

```sh
./x bench
```

That produces `benchmarks/report-<timestamp>.md` with your machine's details,
the Go microbenchmarks, and an end-to-end typing-latency sweep. Reports are not
committed (they are specific to your machine); this folder keeps the tools and
this README.

## What is measured

**Microbenchmarks** (Go, beside the code, run with `go test -bench`):

- `internal/client` — `BenchmarkPredictKey`: the work aft adds per keystroke to
  draw a prediction. This is the number that matters for typing; it is
  microseconds.
- `internal/vt` — `BenchmarkFeed` (emulator parse throughput), `BenchmarkRender`
  (repaint a screen), `BenchmarkSnapshot` (copy-mode snapshot build).
- `internal/proto` — `BenchmarkFrame`: encode and decode one wire frame.

**End-to-end latency** (`benchmarks/latency`): drives the real `aft` client over
a local session at several simulated link delays, with prediction on and off,
and measures how long a typed character takes to appear. Prediction should make
the time independent of the link delay; without it, the time tracks the round
trip. This is the honest demonstration that prediction hides latency.

## Reading the latency table

`predicted` should stay low and flat as the simulated delay grows. `raw` should
rise with the delay (it is the real round trip). The gap is what prediction
hides. Neither can go below your terminal and display latency, which is tens of
milliseconds and outside aft; these numbers isolate aft's own contribution.

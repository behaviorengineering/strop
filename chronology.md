# Chronology

Newest first.

### 2026-09-02 - majordomo - 7c723942d693

- **Did:** Add concurrency package with adaptive in-flight limiter. (#11)
- **Because:** AIMD RunPool for parallel independent LLM units: ramp on fast successes, trip down on timeouts/429/5xx, optional fixed worker mode.
- **In order to:** advance context cursor on default first-parent tape
- **Evidence:** commit 7c723942d6935368f880c587c848949df265835a; files: README.md, concurrency/classify.go, concurrency/config.go, concurrency/doc.go, concurrency/limiter.go, concurrency/limiter_test.go, concurrency/pool.go, doc.go

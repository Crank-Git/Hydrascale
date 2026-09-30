_default:
  @just -l

build *ARGS:
  go build -o hydrascale -v {{ ARGS }} ./cmd/hydrascale

# Install

Install a released binary, or build the binary from source. Both ways put the binary at
`/usr/local/bin/hydrascale`.

## A released binary

Download the archive for the host from the
[GitHub Releases](https://github.com/Crank-Git/Hydrascale/releases) page. Then unpack it
and install the binary:

```bash
tar xzf hydrascale_*.tar.gz
sudo install hydrascale /usr/local/bin/
```

## A build from source

A build from source needs Go 1.26 or later. Clone the repository and build it:

```bash
git clone https://github.com/Crank-Git/Hydrascale.git
cd Hydrascale
go build -o hydrascale ./cmd/hydrascale
sudo install hydrascale /usr/local/bin/
```

`go install` does not work. The module path is `hydrascale`, and the Go module proxy
cannot fetch that path.

## Next step

Read [Quick start](quick-start.md).

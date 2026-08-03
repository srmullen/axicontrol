FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/axicontrold ./cmd/axicontrold

FROM python:3.12-slim

# axicli (the AxiDraw CLI, invoked as a subprocess per ADR-0001) is a Python
# tool requiring a Python runtime and libusb — incompatible with
# distroless/static, so the final stage trades that minimalism for the
# ability to actually run axicli. See ADR-0014.
RUN apt-get update \
    && apt-get install -y --no-install-recommends libusb-1.0-0 \
    && rm -rf /var/lib/apt/lists/* \
    && python -m pip install --no-cache-dir https://cdn.evilmadscientist.com/dl/ad/public/AxiDraw_API.zip

COPY --from=builder /out/axicontrold /axicontrold

ENTRYPOINT ["/axicontrold"]

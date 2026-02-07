# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -trimpath -ldflags "-s -w" -o /out/subconverter-go ./cmd/subconverter-go

FROM scratch

# HTTPS fetch requires CA certs.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/subconverter-go /subconverter-go

EXPOSE 25500

ENTRYPOINT ["/subconverter-go"]
CMD ["-listen","0.0.0.0:25500"]


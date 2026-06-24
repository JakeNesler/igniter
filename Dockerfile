# igniter — homelab host/VM power controller (see main.go for the lifecycle).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/igniter . && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/igniterctl ./cmd/igniterctl

FROM alpine:3.20
# kubectl for node lifecycle (drain/uncordon) via the in-cluster ServiceAccount.
COPY --from=rancher/kubectl:v1.31.5 /bin/kubectl /usr/local/bin/kubectl
COPY --from=build /out/igniter /out/igniterctl /usr/local/bin/
USER 65532:65532
ENTRYPOINT ["igniter"]

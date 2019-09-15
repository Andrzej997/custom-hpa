FROM golang:latest as builder

ADD https://github.com/golang/dep/releases/download/v0.5.4/dep-linux-amd64 /usr/bin/dep

RUN chmod +x /usr/bin/dep

WORKDIR $GOPATH/src/custom-hpa

COPY Gopkg.toml Gopkg.lock ./

RUN dep ensure --vendor-only

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /custom-hpa .


FROM scratch
COPY --from=builder /custom-hpa ./
ENTRYPOINT ["./custom-hpa"]

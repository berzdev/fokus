FROM golang:alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY hetznerFirewall /app/hetznerFirewall
RUN go mod download

COPY *.go ./

RUN go build -o /fokus

#API port
EXPOSE 8080

CMD [ "/fokus" ]

FROM ubuntu:latest

# In production, we should do something more like this:
# https://github.com/docker-library/golang/blob/539882fb23e90d31854a51a773accf8731cf0c9d/1.22/alpine3.20/Dockerfile

RUN apt-get update && apt-get upgrade -y && apt-get install -y wget
# Download and install Go
RUN wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz -O go.tar.gz
RUN tar -C /usr/local -xzf go.tar.gz

# Set Go environment variables
ENV PATH=$PATH:/usr/local/go/bin
RUN mkdir /jogger

COPY go.mod /jogger
# COPY go.sum /jogger
COPY . /jogger
WORKDIR /jogger
RUN go mod download

# Build the application
WORKDIR /jogger/cmd/server
RUN go build -o server

# Copy Keys


# Run the application
CMD ["./server"]

LABEL authors="dustincurrie"

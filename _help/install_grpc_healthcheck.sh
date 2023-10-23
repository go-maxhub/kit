#!/bin/bash

go install github.com/grpc-ecosystem/grpc-health-probe@latest
grpc-health-probe -addr=localhost:8081 -v
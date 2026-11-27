package plugin

//go:generate protoc -I . --go_out=../../internal/plugin/gen --go_opt=paths=source_relative --go-grpc_out=../../internal/plugin/gen --go-grpc_opt=paths=source_relative plugin.proto

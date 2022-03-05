/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// generator is a Go code generator that uses ttrpc.Server and ttrpc.Client.
// Unlike the original gogo version, this doesn't generate serializers for message types and
// let protoc-gen-go handle them.
type generator struct {
	out *protogen.GeneratedFile

	ident struct {
		context     string
		server      string
		client      string
		method      string
		stream      string
		serviceDesc string
		streamDesc  string

		streamServerIdent protogen.GoIdent
		streamClientIdent protogen.GoIdent

		streamServer string
		streamClient string
	}
}

func newGenerator(out *protogen.GeneratedFile) *generator {
	gen := generator{out: out}
	gen.ident.context = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "context",
		GoName:       "Context",
	})
	gen.ident.server = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "Server",
	})
	gen.ident.client = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "Client",
	})
	gen.ident.method = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "Method",
	})
	gen.ident.stream = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "Stream",
	})
	gen.ident.serviceDesc = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "ServiceDesc",
	})
	gen.ident.streamDesc = out.QualifiedGoIdent(protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "StreamDesc",
	})

	gen.ident.streamServerIdent = protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "StreamServer",
	}
	gen.ident.streamClientIdent = protogen.GoIdent{
		GoImportPath: "github.com/containerd/ttrpc",
		GoName:       "ClientStream",
	}
	gen.ident.streamServer = out.QualifiedGoIdent(gen.ident.streamServerIdent)
	gen.ident.streamClient = out.QualifiedGoIdent(gen.ident.streamClientIdent)
	return &gen
}

func generate(plugin *protogen.Plugin, input *protogen.File) error {
	file := plugin.NewGeneratedFile(input.GeneratedFilenamePrefix+"_ttrpc.pb.go", input.GoImportPath)
	file.P("// Code generated by protoc-gen-go-ttrpc. DO NOT EDIT.")
	file.P("// source: ", input.Desc.Path())
	file.P("package ", input.GoPackageName)

	gen := newGenerator(file)
	for _, service := range input.Services {
		gen.genService(service)
	}
	return nil
}

func (gen *generator) genService(service *protogen.Service) {
	fullName := service.Desc.FullName()
	p := gen.out

	var methods []*protogen.Method
	var streams []*protogen.Method

	serviceName := service.GoName + "Service"
	p.P("type ", serviceName, " interface{")
	for _, method := range service.Methods {
		var sendArgs, retArgs string
		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			streams = append(streams, method)
			sendArgs = fmt.Sprintf("%s_%sServer", service.GoName, method.GoName)
			if !method.Desc.IsStreamingClient() {
				sendArgs = fmt.Sprintf("*%s, %s", p.QualifiedGoIdent(method.Input.GoIdent), sendArgs)
			}
			if method.Desc.IsStreamingServer() {
				retArgs = "error"
			} else {
				retArgs = fmt.Sprintf("(*%s, error)", p.QualifiedGoIdent(method.Output.GoIdent))
			}
		} else {
			methods = append(methods, method)
			sendArgs = fmt.Sprintf("*%s", p.QualifiedGoIdent(method.Input.GoIdent))
			retArgs = fmt.Sprintf("(*%s, error)", p.QualifiedGoIdent(method.Output.GoIdent))
		}
		p.P(method.GoName, "(", gen.ident.context, ", ", sendArgs, ") ", retArgs)
	}
	p.P("}")
	p.P()

	for _, method := range streams {
		structName := strings.ToLower(service.GoName) + method.GoName + "Server"

		p.P("type ", service.GoName, "_", method.GoName, "Server interface {")
		if method.Desc.IsStreamingServer() {
			p.P("Send(*", method.Output.GoIdent, ") error")
		}
		if method.Desc.IsStreamingClient() {
			p.P("Recv() (*", method.Input.GoIdent, ", error)")

		}
		p.P(gen.ident.streamServer)
		p.P("}")
		p.P()

		p.P("type ", structName, " struct {")
		p.P(gen.ident.streamServer)
		p.P("}")
		p.P()

		if method.Desc.IsStreamingServer() {
			p.P("func (x *", structName, ") Send(m *", method.Output.GoIdent, ") error {")
			p.P("return x.StreamServer.SendMsg(m)")
			p.P("}")
			p.P()
		}

		if method.Desc.IsStreamingClient() {
			p.P("func (x *", structName, ") Recv() (*", method.Input.GoIdent, ", error) {")
			p.P("m := new(", method.Input.GoIdent, ")")
			p.P("if err := x.StreamServer.RecvMsg(m); err != nil {")
			p.P("return nil, err")
			p.P("}")
			p.P("return m, nil")
			p.P("}")
			p.P()
		}
	}

	// registration method
	p.P("func Register", serviceName, "(srv *", gen.ident.server, ", svc ", serviceName, "){")
	p.P(`srv.RegisterService("`, fullName, `", &`, gen.ident.serviceDesc, "{")
	if len(methods) > 0 {
		p.P(`Methods: map[string]`, gen.ident.method, "{")
		for _, method := range methods {
			p.P(`"`, method.GoName, `": func(ctx `, gen.ident.context, ", unmarshal func(interface{}) error)(interface{}, error){")
			p.P("var req ", method.Input.GoIdent)
			p.P("if err := unmarshal(&req); err != nil {")
			p.P("return nil, err")
			p.P("}")
			p.P("return svc.", method.GoName, "(ctx, &req)")
			p.P("},")
		}
		p.P("},")
	}
	if len(streams) > 0 {
		p.P(`Streams: map[string]`, gen.ident.stream, "{")
		for _, method := range streams {
			p.P(`"`, method.GoName, `": {`)
			p.P(`Handler: func(ctx `, gen.ident.context, ", stream ", gen.ident.streamServer, ") (interface{}, error) {")

			structName := strings.ToLower(service.GoName) + method.GoName + "Server"
			var sendArg string
			if !method.Desc.IsStreamingClient() {
				sendArg = "m, "
				p.P("m := new(", method.Input.GoIdent, ")")
				p.P("if err := stream.RecvMsg(m); err != nil {")
				p.P("return nil, err")
				p.P("}")
			}
			if method.Desc.IsStreamingServer() {
				p.P("return nil, svc.", method.GoName, "(ctx, ", sendArg, "&", structName, "{stream})")
			} else {
				p.P("return svc.", method.GoName, "(ctx, ", sendArg, "&", structName, "{stream})")

			}
			p.P("},")
			if method.Desc.IsStreamingClient() {
				p.P("StreamingClient: true,")
			} else {
				p.P("StreamingClient: false,")
			}
			if method.Desc.IsStreamingServer() {
				p.P("StreamingServer: true,")
			} else {
				p.P("StreamingServer: false,")
			}
			p.P("},")
		}
		p.P("},")
	}
	p.P("})")
	p.P("}")
	p.P()

	clientType := service.GoName + "Client"

	// For consistency with ttrpc 1.0 without streaming, just use
	// the service name if no streams are defined
	clientInterface := serviceName
	if len(streams) > 0 {
		clientInterface = clientType
		// Stream client interfaces are different than the server interface
		p.P("type ", clientInterface, " interface{")
		for _, method := range service.Methods {
			if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
				streams = append(streams, method)
				var sendArg string
				if !method.Desc.IsStreamingClient() {
					sendArg = fmt.Sprintf("*%s, ", p.QualifiedGoIdent(method.Input.GoIdent))
				}
				p.P(method.GoName,
					"(", gen.ident.context, ", ", sendArg,
					") (", service.GoName, "_", method.GoName, "Client, error)")
			} else {
				methods = append(methods, method)
				p.P(method.GoName,
					"(", gen.ident.context, ", ",
					"*", method.Input.GoIdent, ")",
					"(*", method.Output.GoIdent, ", error)")
			}
		}
		p.P("}")
		p.P()
	}

	clientStructType := strings.ToLower(clientType[:1]) + clientType[1:]
	p.P("type ", clientStructType, " struct{")
	p.P("client *", gen.ident.client)
	p.P("}")
	p.P("func New", clientType, "(client *", gen.ident.client, ")", clientInterface, "{")
	p.P("return &", clientStructType, "{")
	p.P("client:client,")
	p.P("}")
	p.P("}")
	p.P()

	for _, method := range service.Methods {
		var sendArg string
		if !method.Desc.IsStreamingClient() {
			sendArg = ", req *" + gen.out.QualifiedGoIdent(method.Input.GoIdent)
		}

		intName := service.GoName + "_" + method.GoName + "Client"
		var retArg string
		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			retArg = intName
		} else {
			retArg = "*" + gen.out.QualifiedGoIdent(method.Output.GoIdent)
		}

		p.P("func (c *", clientStructType, ") ", method.GoName,
			"(ctx ", gen.ident.context, "", sendArg, ") ",
			"(", retArg, ", error) {")

		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			var streamingClient, streamingServer, req string
			if method.Desc.IsStreamingClient() {
				streamingClient = "true"
				req = "nil"
			} else {
				streamingClient = "false"
				req = "req"
			}
			if method.Desc.IsStreamingServer() {
				streamingServer = "true"
			} else {
				streamingServer = "false"
			}
			p.P("stream, err := c.client.NewStream(ctx, &", gen.ident.streamDesc, "{")
			p.P("StreamingClient: ", streamingClient, ",")
			p.P("StreamingServer: ", streamingServer, ",")
			p.P("}, ", `"`+fullName+`", `, `"`+method.GoName+`", `, req, `)`)
			p.P("if err != nil {")
			p.P("return nil, err")
			p.P("}")

			structName := strings.ToLower(service.GoName) + method.GoName + "Client"

			p.P("x := &", structName, "{stream}")

			p.P("return x, nil")
			p.P("}")
			p.P()

			// Create interface
			p.P("type ", intName, " interface {")
			if method.Desc.IsStreamingClient() {
				p.P("Send(*", method.Input.GoIdent, ") error")
			}
			if method.Desc.IsStreamingServer() {
				p.P("Recv() (*", method.Output.GoIdent, ", error)")
			} else {
				p.P("CloseAndRecv() (*", method.Output.GoIdent, ", error)")
			}

			p.P(gen.ident.streamClient)
			p.P("}")
			p.P()

			// Create struct
			p.P("type ", structName, " struct {")
			p.P(gen.ident.streamClient)
			p.P("}")
			p.P()

			if method.Desc.IsStreamingClient() {
				p.P("func (x *", structName, ") Send(m *", method.Input.GoIdent, ") error {")
				p.P("return x.", gen.ident.streamClientIdent.GoName, ".SendMsg(m)")
				p.P("}")
				p.P()
			}

			if method.Desc.IsStreamingServer() {
				p.P("func (x *", structName, ") Recv() (*", method.Output.GoIdent, ", error) {")
				p.P("m := new(", method.Output.GoIdent, ")")
				p.P("if err := x.ClientStream.RecvMsg(m); err != nil {")
				p.P("return nil, err")
				p.P("}")
				p.P("return m, nil")
				p.P("}")
				p.P()
			} else {
				p.P("func (x *", structName, ") CloseAndRecv() (*", method.Output.GoIdent, ", error) {")
				p.P("if err := x.ClientStream.CloseSend(); err != nil {")
				p.P("return nil, err")
				p.P("}")
				p.P("m := new(", method.Output.GoIdent, ")")
				p.P("if err := x.ClientStream.RecvMsg(m); err != nil {")
				p.P("return nil, err")
				p.P("}")
				p.P("return m, nil")
				p.P("}")
				p.P()
			}
		} else {
			p.P("var resp ", method.Output.GoIdent)
			p.P(`if err := c.client.Call(ctx, "`, fullName, `", "`, method.Desc.Name(), `", req, &resp); err != nil {`)
			p.P("return nil, err")
			p.P("}")
			p.P("return &resp, nil")
			p.P("}")
			p.P()
		}
	}
}

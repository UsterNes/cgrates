// +build integration

/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package cores

import (
	"bytes"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc/jsonrpc"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	sessions2 "github.com/cgrates/cgrates/sessions"

	"github.com/cenkalti/rpc2"

	"github.com/cgrates/cgrates/config"

	"github.com/cgrates/cgrates/engine"

	"github.com/cgrates/cgrates/utils"
)

var (
	server *Server

	sTestsServer = []func(t *testing.T){
		testServeGOBPortFail,
		testServeJSON,
		testServeJSONFail,
		testServeJSONFailRpcEnabled,
		testServeGOB,
		testServeHHTPPass,
		testServeHHTPPassUseBasicAuth,
		testServeHHTPEnableHttp,
		testServeHHTPFail,
		testServeHHTPFailEnableRpc,
		testServeBiJSON,
		testServeBiJSONEmptyBiRPCServer,
		testServeBiJSONInvalidPort,
		testServeGOBTLS,
		testServeJSONTls,
		testServeCodecTLSErr,
		testLoadTLSConfigErr,
		testServeHTTPTLS,
		testServeHTTPTLSWithBasicAuth,
		testServeHTTPTLSError,
		testServeHTTPTLSHttpNotEnabled,
		testHandleRequest,
		testBiRPCRegisterName,
	}
)

func TestServerIT(t *testing.T) {
	utils.Logger.SetLogLevel(7)
	for _, test := range sTestsServer {
		log.SetOutput(ioutil.Discard)
		t.Run("Running IT serve tests", test)
	}
}

type mockRegister struct{}

func (robj *mockRegister) Ping(in string, out *string) error {
	*out = utils.Pong
	return nil
}

type mockListener struct {
	p1   net.Conn
	call bool
}

func (mkL *mockListener) Accept() (net.Conn, error) {
	if !mkL.call {
		mkL.call = true
		return mkL.p1, nil
	}
	return nil, utils.ErrDisconnected
}

func (mkL *mockListener) Close() error   { return mkL.p1.Close() }
func (mkL *mockListener) Addr() net.Addr { return nil }

func testHandleRequest(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	cfgDflt.CoreSCfg().CapsStatsInterval = 1
	caps := engine.NewCaps(0, utils.MetaBusy)
	rcv := NewServer(caps)

	rcv.rpcEnabled = true

	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:2080/json_rpc",
		bytes.NewBuffer([]byte("1")))
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	rcv.handleRequest(w, req)
	if w.Body.String() != utils.EmptyString {
		t.Errorf("Expected: %q ,received: %q", utils.EmptyString, w.Body.String())
	}

	rcv.StopBiRPC()
}

func testServeJSON(t *testing.T) {
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	buff := new(bytes.Buffer)
	log.SetOutput(buff)

	go server.ServeJSON(":88845", shdChan)
	runtime.Gosched()

	expected := "listen tcp: address 88845: invalid port"
	if rcv := buff.String(); !strings.Contains(rcv, expected) {
		t.Errorf("Expected %+v, received %+v", expected, rcv)
	}

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeJSONFail(t *testing.T) {
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	p1, p2 := net.Pipe()
	l := &mockListener{
		p1: p1,
	}
	go server.accept(l, utils.JSONCaps, newCapsJSONCodec, shdChan)
	runtime.Gosched()
	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
	p2.Close()
	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeJSONFailRpcEnabled(t *testing.T) {
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()
	server.rpcEnabled = false

	go server.serveCodec(":9999", utils.JSONCaps, newCapsJSONCodec, shdChan)
	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeGOB(t *testing.T) {
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	go server.ServeGOB(":27697", shdChan)
	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeHHTPPass(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	go server.ServeHTTP(
		":6555",
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeHHTPPassUseBasicAuth(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	go server.ServeHTTP(
		":56432",
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		!cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeHHTPEnableHttp(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	go server.ServeHTTP(
		":45779",
		utils.EmptyString,
		utils.EmptyString,
		!cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	runtime.Gosched()

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeHHTPFail(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()

	go server.ServeHTTP(
		"invalid_portt_format",
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	runtime.Gosched()

	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
	server.StopBiRPC()
}

func testServeHHTPFailEnableRpc(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	shdChan := utils.NewSyncedChan()
	server.rpcEnabled = false

	go server.ServeHTTP(":1000",
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	shdChan.CloseOnce()
	server.StopBiRPC()
}

func testServeBiJSON(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	server.birpcSrv = rpc2.NewServer()

	data := engine.NewInternalDB(nil, nil, true)
	dm := engine.NewDataManager(data, cfgDflt.CacheCfg(), nil)

	sessions := sessions2.NewSessionS(cfgDflt, dm, nil)

	go func() {
		if err := server.ServeBiJSON(":3434", sessions.OnBiJSONConnect, sessions.OnBiJSONDisconnect); err != nil {
			t.Error(err)
		}
	}()
	runtime.Gosched()
}

func testServeBiJSONEmptyBiRPCServer(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	data := engine.NewInternalDB(nil, nil, true)
	dm := engine.NewDataManager(data, cfgDflt.CacheCfg(), nil)

	sessions := sessions2.NewSessionS(cfgDflt, dm, nil)

	expectedErr := "BiRPCServer should not be nil"
	go func() {
		if err := server.ServeBiJSON(":3430", sessions.OnBiJSONConnect, sessions.OnBiJSONDisconnect); err == nil || err.Error() != "BiRPCServer should not be nil" {
			t.Errorf("Expected %+v, received %+v", expectedErr, err)
		}
	}()

	runtime.Gosched()
}

func testServeBiJSONInvalidPort(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))
	server.birpcSrv = rpc2.NewServer()

	data := engine.NewInternalDB(nil, nil, true)
	dm := engine.NewDataManager(data, cfgDflt.CacheCfg(), nil)

	sessions := sessions2.NewSessionS(cfgDflt, dm, nil)

	expectedErr := "listen tcp: address invalid_port_format: missing port in address"
	if err := server.ServeBiJSON("invalid_port_format", sessions.OnBiJSONConnect,
		sessions.OnBiJSONDisconnect); err == nil || err.Error() != expectedErr {
		t.Errorf("Expected %+v, received %+v", expectedErr, err)
	}

	server.StopBiRPC()
}

func testServeGOBTLS(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	go server.ServeGOBTLS(
		":34476",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		4,
		cfgDflt.TLSCfg().ServerName,
		shdChan,
	)
	runtime.Gosched()

	server.StopBiRPC()
}

func testServeJSONTls(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	go server.ServeJSONTLS(
		":64779",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		4,
		cfgDflt.TLSCfg().ServerName,
		shdChan,
	)
	runtime.Gosched()
}

func testServeGOBPortFail(t *testing.T) {
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	buff := new(bytes.Buffer)
	log.SetOutput(buff)

	go server.serveCodecTLS(
		"34776",
		utils.GOBCaps,
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		4,
		"Server_name",
		newCapsGOBCodec,
		shdChan,
	)
	runtime.Gosched()
	select {
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	case <-shdChan.Done():
	}
	expected := "listen tcp: address 34776: missing port in address when listening"
	if rcv := buff.String(); !strings.Contains(rcv, expected) {
		t.Errorf("Expected %+v, received %+v", expected, rcv)
	}

	log.SetOutput(os.Stderr)
}

func testServeCodecTLSErr(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	//if rpc is not enabled, won t be able to serve
	server.rpcEnabled = false
	server.serveCodecTLS("13567",
		utils.GOBCaps,
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		4,
		cfgDflt.TLSCfg().ServerName,
		newCapsGOBCodec,
		shdChan)

	//unable to load TLS config when there is an inexisting server certificate file
	server.rpcEnabled = true
	server.serveCodecTLS("13567",
		utils.GOBCaps,
		"/usr/share/cgrates/tls/inexisting_cert",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		4,
		cfgDflt.TLSCfg().ServerName,
		newCapsGOBCodec,
		shdChan)

	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
}

func testLoadTLSConfigErr(t *testing.T) {
	flPath := "/tmp/testLoadTLSConfigErr1"
	if err := os.MkdirAll(flPath, 0777); err != nil {
		t.Error(err)
	}
	file, err := os.Create(path.Join(flPath, "file.txt"))
	if err != nil {
		t.Error(err)
	}
	file.Write([]byte(`
TEST
`))
	file.Close()

	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	expectedErr := "Cannot append certificate authority"
	if _, err := loadTLSConfig(
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		path.Join(flPath, "file.txt"),
		0,
		utils.EmptyString); err == nil || err.Error() != expectedErr {
		t.Errorf("Expected %+v, received %+v", expectedErr, err)
	}

	expectedErr = "open /tmp/testLoadTLSConfigErr1/file1.txt: no such file or directory"
	if _, err := loadTLSConfig(
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		path.Join(flPath, "file1.txt"),
		0,
		utils.EmptyString); err == nil || err.Error() != expectedErr {
		t.Errorf("Expected %+v, received %+v", expectedErr, err)
	}
	if err := os.Remove(path.Join(flPath, "file.txt")); err != nil {
		t.Error(err)
	} else if err := os.Remove(flPath); err != nil {
		t.Error(err)
	}
}

func testServeHTTPTLS(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	//cannot serve HHTPTls when rpc is not enabled
	server.rpcEnabled = false
	server.ServeHTTPTLS(
		"17789",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		cfgDflt.TLSCfg().ServerPolicy,
		cfgDflt.TLSCfg().ServerName,
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	//Invalid port address
	server.rpcEnabled = true
	go server.ServeHTTPTLS(
		"17789",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		cfgDflt.TLSCfg().ServerPolicy,
		cfgDflt.TLSCfg().ServerName,
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)
	runtime.Gosched()

	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
}

func testServeHTTPTLSWithBasicAuth(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	//Invalid port address
	server.rpcEnabled = true
	go server.ServeHTTPTLS(
		"57235",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		cfgDflt.TLSCfg().ServerPolicy,
		cfgDflt.TLSCfg().ServerName,
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		!cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)
	runtime.Gosched()

	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
}

func testServeHTTPTLSError(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	//Invalid port address
	go server.ServeHTTPTLS(
		"57235",
		"/usr/share/cgrates/tls/inexisting_file",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		cfgDflt.TLSCfg().ServerPolicy,
		cfgDflt.TLSCfg().ServerName,
		cfgDflt.HTTPCfg().HTTPJsonRPCURL,
		cfgDflt.HTTPCfg().HTTPWSURL,
		!cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)
	runtime.Gosched()

	_, ok := <-shdChan.Done()
	if ok {
		t.Errorf("Expected to be close")
	}
}

func testServeHTTPTLSHttpNotEnabled(t *testing.T) {
	cfgDflt := config.NewDefaultCGRConfig()
	caps := engine.NewCaps(100, utils.MetaBusy)
	server = NewServer(caps)
	server.RpcRegister(new(mockRegister))

	shdChan := utils.NewSyncedChan()

	server.httpEnabled = false
	go server.ServeHTTPTLS(
		"17789",
		"/usr/share/cgrates/tls/server.crt",
		"/usr/share/cgrates/tls/server.key",
		"/usr/share/cgrates/tls/ca.crt",
		cfgDflt.TLSCfg().ServerPolicy,
		cfgDflt.TLSCfg().ServerName,
		utils.EmptyString,
		utils.EmptyString,
		cfgDflt.HTTPCfg().HTTPUseBasicAuth,
		cfgDflt.HTTPCfg().HTTPAuthUsers,
		shdChan)

	shdChan.CloseOnce()
}

func testBiRPCRegisterName(t *testing.T) {
	caps := engine.NewCaps(0, utils.MetaBusy)
	server := NewServer(caps)

	handler := func(method *rpc2.Client, args *interface{}, reply *interface{}) error {
		return nil
	}
	go server.BiRPCRegisterName(utils.APIerSv1Ping, handler)
	runtime.Gosched()

	server.StopBiRPC()
}
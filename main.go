package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/status-im/keycard-go/hexutils"
	"golang.org/x/term"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type JsonRpcRequest struct {
	Version string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
	Id      *int          `json:"id,omitempty"`
}

func serveSiege(log log.Logger, cl *http.Client, targetURL string, cannonPath string, res http.ResponseWriter, req *http.Request) {
	// copy the body
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, req.Body); err != nil {
		res.WriteHeader(400)
		res.Write([]byte("failed to read request"))
		log.Error("failed to read request", "err", err)
		return
	}

	// Parse body
	var rpcReq JsonRpcRequest
	if err := json.Unmarshal(buf.Bytes(), &rpcReq); err != nil {
		res.WriteHeader(400)
		res.Write([]byte("failed to parse request"))
		log.Error("failed to parse request", "err", err)
		return
	}

	// forward the request
	innerReq, err := http.NewRequest("POST", targetURL, &buf)
	if err != nil {
		res.WriteHeader(500)
		res.Write([]byte("failed to build inner request"))
		log.Error("failed to build inner request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	innerRes, err := cl.Do(innerReq)
	if err != nil {
		res.WriteHeader(500)
		res.Write([]byte("failed to complete inner request"))
		log.Error("failed to complete inner request", "err", err)
		return
	}

	// Now check if we should run cannon
	if rpcReq.Method == "test_importRawBlock" {
		var block types.Block
		if err := rlp.DecodeBytes(hexutils.HexToBytes(rpcReq.Params[0].(string)), &block); err != nil {
			log.Debug("failed to parse block RLP, maybe an intentionally invalid test block? skipping", "err", err)
		} else {
			// run cannon with arguments "test", <block num>, <block hash>, <state root>, <receipt root>
			cmd := exec.Command(cannonPath,
				"test",
				fmt.Sprintf("%d", block.NumberU64()),
				block.Hash().String(),
				block.Root().String(),
				block.ReceiptHash().String(),
			)
			if err := cmd.Start(); err != nil {
				res.WriteHeader(500)
				res.Write([]byte("failed to start cannon"))
				log.Error("failed to start cannon", "err", err)
				return
			}
			stderr, _ := cmd.StderrPipe()
			go func() {
				stderrScanner := bufio.NewScanner(stderr)
				for stderrScanner.Scan() {
					log.Error(stderrScanner.Text())
				}
				if err := stderrScanner.Err(); err != nil {
					log.Error("std err stopped", "err", err)
				}
			}()
			stdout, _ := cmd.StdoutPipe()
			go func() {
				stdoutScanner := bufio.NewScanner(stdout)
				for stdoutScanner.Scan() {
					log.Error(stdoutScanner.Text())
				}
				if err := stdoutScanner.Err(); err != nil {
					log.Error("std out stopped", "err", err)
				}
			}()
			if err := cmd.Wait(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					res.WriteHeader(500)
					res.Write([]byte(fmt.Sprintf("cannon exited with error %d", exitErr.ExitCode())))
					log.Error("cannon exited with error", "code", exitErr.ExitCode())
					return
				} else {
					cmd.Process.Kill()
					res.WriteHeader(500)
					res.Write([]byte("failed to wait for cannon, killing it"))
					log.Error("failed to wait for cannon", "err", err)
				}
			}
		}
	}

	hdr := res.Header()
	for k, v := range innerRes.Header {
		hdr[k] = v
	}
	res.WriteHeader(innerRes.StatusCode)
	if _, err := io.Copy(res, innerRes.Body); err != nil {
		res.WriteHeader(500)
		res.Write([]byte("failed to copy over inner response"))
		log.Error("failed to copy over inner response", "err", err)
		return
	}
}

func main() {
	logColor := flag.Bool("log.color", term.IsTerminal(int(os.Stdout.Fd())), "colored terminal output")
	logFormat := flag.String("log.format", "text", "text or json format logging")
	logLvl := flag.String("log.level", "info", "log level")
	nodeAddr := flag.String("node.addr", "http://127.0.0.1:8545", "http address of surrounded node")
	siegeAddr := flag.String("node.addr", "http://127.0.0.1:9000", "http address of siege")
	cannonPath := flag.String("cannon", "../cannon", "cannon binary path")
	flag.Parse()

	var format log.Format
	switch *logFormat {
	case "json":
		format = log.JSONFormat()
	default:
		format = log.TerminalFormat(*logColor)
	}
	lvl, err := log.LvlFromString(*logLvl)
	if err != nil {
		lvl = log.LvlInfo
	}
	handler := log.StreamHandler(os.Stdout, format)
	handler = log.SyncHandler(handler)
	log.LvlFilterHandler(lvl, handler)

	log := log.New()
	log.SetHandler(handler)

	cl := &http.Client{Timeout: time.Second * 60}
	handle := func(res http.ResponseWriter, req *http.Request) {
		serveSiege(log, cl, *nodeAddr, *cannonPath, res, req)
	}

	// start server
	http.HandleFunc("/", handle)

	if err := http.ListenAndServe(*siegeAddr, nil); err != nil {
		log.Error("http server stopped", "err", err)
	}
}

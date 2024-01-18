---
title: golang 1.20 http2源码解读
date: 2024-01-01 14:05:46
tags: [http2, golang, bio, nio, aio]
---

## 前言
搞清楚linux进程调度后，下一步就是网络。从最常见的http开始，考虑到java源码的冗长以及jvm的naive不透明，不如从golang入手。
本次解读基于golang 1.20。

### Http 1.1
Http1.1应该是web协议中最简单的一个。一句话概括，给每个连接分配一个goroutine，goroutine负责读取请求-处理-发送响应，然后关闭连接或者在这个连接上循环做读取请求-处理-发送响应。
这整个过程都是同步且阻塞的。如图：

![](http1.1.png)

源码也非常简单：
#### 主goroutine
```go
server.ListenAndServe()
    ln, err := net.Listen("tcp", addr)
    srv.Serve(ln)
        func (srv *Server) Serve(l net.Listener) error {
            for {
                rw, e := l.Accept()
                c, err := srv.newConn(rw)
                go c.serve()
            }
        }
```

#### 每个连接一个goroutine
```go
// read request
w, err := c.readRequest(ctx)
    // read request header
    mimeHeader, err := tp.ReadMIMEHeader()
// handle request
serverHandler{c.server}.ServeHTTP(w, w.req)
	//read body and handle
	io.readAll(req.Body)
	writer.Write([]byte("Hello World"))
        w.WriteHeader(StatusOK)
		// write response body
		w.w.Write(dataB)
// flush response
w.finishRequest()
    w.w.Flush()
```




---
title: golang 1.20 http2源码解读
date: 2024-01-01 14:05:46
tags: [http2, golang, bio, nio, aio]
---

##前言
搞清楚linux进程调度后，下一步就是网络。从最常见的http开始，考虑到java源码的冗长以及jvm的naive不透明，不如从golang入手。http1.x的解读就不写了。
golang由于goroutine的存在，http1.x的实现也是bio的，没什么含金量。不如直接来看看http2。

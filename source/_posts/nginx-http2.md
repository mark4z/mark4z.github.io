---
title: 谈谈Http2-为什么nginx在http2下的表现远弱于http1.1
date: 2023-04-02 14:05:46
tags: [http2, nginx, benchmark]
---

## 背景

本篇文章的灵感来源于不久前的一次压测，接口的TPS在100并发下不足500，且延时超过500MS，这个接口的性能远远低于我们的预期。
对应的接口是一个极为简单的接口，只做了两件事情：
![](api.png)

1. 从数据库中读取一条数据，做了一些简单的处理
2. 进行一次HTTP调用

那么话不多说，开始从内到外几个排查：

- [x] DB的CPU、内存、磁盘IO、网络IO等指标一切正常：排除慢sql相关问题
- [x] POD，即容器的CPU、内存、网络IO等指标一切正常：排除容器资源不足问题
- [x] JVM的Heap Size即堆内存、GC、线程数等指标一切正常：排除内存泄漏、GC问题
- [ ] 利用[Arshas](https://arthas.aliyun.com/doc)或其他监测工具生成火焰图或接口耗时分析图，发现接口耗时主要集中第二步

## Stream Reset Exception

上文提到发现接口的主要耗时集中在第二步，那么我们就从第二步开始分析：

1. 排查发现，该Http调用使用的是Okhttp3, 且使用的是默认的连接池，最多复用5个连接，看起来有点太少了，
   但后续调整到100后发现性能并没有提升，所以这个问题基本可以排除。（当然连接池确实过小，但是与此次的性能问题并无关系）

```java
@Configuration
public class RestTemplateConfig {

    @Bean
    public RestTemplate restTemplate() {
        return new RestTemplate(new OkHttp3ClientHttpRequestFactory());
    }
}
```

2. 开启Okhttp的日志，发现有大量的Stream Reset Exception，出现该异常的概率大概在10%

```java
Caused by: okhttp3.internal.framed.StreamResetException: stream was reset: CANCEL
        at okhttp3.internal.framed.FramedStream.getResponseHeaders(FramedStream.java:145)
        at okhttp3.internal.http.Http2xStream.readResponseHeaders(Http2xStream.java:149)
        at okhttp3.internal.http.HttpEngine.readNetworkResponse(HttpEngine.java:775)
        at okhttp3.internal.http.HttpEngine.access$200(HttpEngine.java:86)
        at okhttp3.internal.http.HttpEngine$NetworkInterceptorChain.proceed(HttpEngine.java:760)
        at okhttp3.internal.http.HttpEngine.readResponse(HttpEngine.java:613)
        at okhttp3.RealCall.getResponse(RealCall.java:244)
        at okhttp3.RealCall$ApplicationInterceptorChain.proceed(RealCall.java:201)
        at okhttp3.RealCall.getResponseWithInterceptorChain(RealCall.java:163)
        at okhttp3.RealCall.execute(RealCall.java:57)
```

至此，上游API的同学已经传来了消息，他们的接口一切正常，响应时间99%在20ms以内，那么问题基本可以收敛至从发起调用到接收到响应的这段时间，即从Okhttp到上游API的这段时间。

## Http2

通过上文的Stream Reset Exception可以发现，该Http调用使用的是h2协议，与平时的内部RPC并不一样。这也是该问题第一次出现的原因。

### 什么是Http2协议
{% blockquote%}
HTTP/2 是 HTTP 协议自 1999 年 HTTP 1.1 发布后的首个更新，主要基于 SPDY 协议。1 它具有以下几个特点：

- 二进制分帧：HTTP/2 采用二进制格式传输数据，而非 HTTP 1.x 的文本格式。
- 多路复用：HTTP/2 中，同域名下所有通信都在单个连接上完成，该连接可以承载任意数量的双向数据流。
- 头部压缩：HTTP/2 对消息头采用 HPACK 进行压缩传输，能够节省消息头占用的网络流量。
- 服务器推送：服务端可以在发送页面 HTML 时主动推送其他资源，而不用等到浏览器解析到相应位置，发起请求再响应。

这些特点使得 HTTP/2 在性能上有了很大的提升。
{% endblockquote %}

`在这里解释一下为何上文提到，即使连接池大小只有5，但是对于这次的场景并不是瓶颈，因为在理想情况下，无论并发是多少，http2由于支持多路复用，所以并发调用仅需一条tcp连接,根本不需要连接池！！！`

## 为什么会出现Stream Reset Exception

至此，调用方的问题已经排查完毕，那么问题就转移到了上游API，我们来看下上游API背后的拓扑结构：
![](third-api.png)
通过对上游API的压测，发现从API Gateway->POD这一侧压测接口正常，但是从Reverse Proxy->API Gateway->POD这一侧压测接口就会出现大量Stream
Reset Exception，这也就是为什么在上游API的同学看来，他们的接口一切正常，而我们却发现接口的响应时间超过500ms的原因。

在此我准备了最小复现问题的demo，可以直接clone下来。
```bash
git clone https://github.com/mark4z/rpc-benchmark.git
cd rpc-benchmark
docker-compose up
```

{% blockquote%}
#### 一个小巧但好用的轻量压测工具  https://github.com/mark4z/hey

对于一个后端同学来说，jemter太重了，wrk刚刚好，但是不支持Http2！！！

在这里，介绍一下hey，支持wrk的所有功能，同时支持Http2，我还为这个工具增加了实时显示压测进度的能力。
安装起来很容易，如果你本地有go环境，执行下列命令即可（我有时间会简化下安装方式的，下次一定！）：
<img src="hey.gif" width="60%">

```bash 
go install github.com/mark4z/hey@latest
```
{% endblockquote %}
#### 通过Http1.1协议压测对应接口
```bash
hey -c 100 -z 30s -m POST -d "1" https://localhost:9998/delay
```
```log
Summary:
  Total:	30.0101 secs
  Slowest:	0.2814 secs
  Fastest:	0.0012 secs
  Average:	0.0147 secs
  Requests/sec:	9899.7792
  
  New connection:	100
  
Response time histogram:
  0.029 [190818]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.057 [11493]	|■■
```
#### 通过Http2协议压测对应接口
```bash
hey -c 100 -z 30s -m POST -d "1" -h2  https://localhost:9998/delay
```
```log
Summary:
  Total:	30.0161 secs
  Slowest:	0.3233 secs
  Fastest:	0.0014 secs
  Average:	0.0254 secs
  Requests/sec:	4046.8258
  
  New connection:	4495

Response time histogram:
  0.034 [89301]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.066 [21109]	|■■■■■■■■■
  0.098 [5277]	|■■
Error distribution:
  [4117]	Post "https://localhost:9998/delay": http2: Transport: cannot retry err [http2: Transport received Server's graceful shutdown GOAWAY] after Request.Body was written; define Request.GetBody to avoid this error
```
我们成功复现了问题，可以看到在用nginx作为反向代理时，http2的性能表现居然比http1.1差了一倍！！！同时，在压测过程中，也出现了上文提到的Stream Reset Exception，即服务端发送了GOAWAY主动关闭了连接，从压测结果来看也能看到New connection:	4495，是http1.1的数十倍。
众所周知，创建https连接是非常耗时的，那么问题就在这里了。

进一步分析，我们来打开hey的debug模式，看看具体的请求过程：
```bash
export GODEBUG=http2debug=2
hey -c 1 -n 1001 -m POST -d "0" -h2  https://localhost:9998/delay
````
下面是hey的debug日志,我们来逐行分析一下：
```log
//stream=1999 每个stream都有一个唯一的id，对应一次请求与响应，由于客户端发送的stream id只能是奇数，所以这里代表第1000个请求
2023/04/02 20:42:25 http2: Framer 0x1400021a0e0: wrote DATA flags=END_STREAM stream=1999 len=1 data="0"
...
2023/04/02 20:42:25 http2: Framer 0x1400021a0e0: read HEADERS flags=END_STREAM|END_HEADERS stream=1999 len=39
...
2023/04/02 20:42:25 http2: Transport received HEADERS flags=END_STREAM|END_HEADERS stream=1999 len=39
//这里hey尝试下次请求时，发现连接池中没有可用的连接了，所以创建了一个新的连接
2023/04/02 20:42:25 http2: Transport failed to get client conn for localhost:9998: http2: no cached connection was available
2023/04/02 20:42:25 http2: Transport failed to get client conn for localhost:9998: http2: no cached connection was available
2023/04/02 20:42:25 http2: Transport readFrame error on conn 0x1400022a000: (*errors.errorString) EOF
2023/04/02 20:42:25 http2: Transport creating client conn 0x140001ac180 to [::1]:9998
...
2023/04/02 20:42:25 http2: Transport encoding header ":path" = "/delay"
2023/04/02 20:42:25 http2: Transport encoding header ":scheme" = "https"
// 这里的stream 1，也就是新连接的第一个请求
2023/04/02 20:42:25 http2: Framer 0x1400021a2a0: wrote HEADERS flags=END_HEADERS stream=1 len=47
...
```
从日志中可以看出，当hey发送了1000个请求后，服务端主动关闭了连接。

## NGINX的参数问题
通过上文我们发现了NGINX作为上游频繁关闭http2的连接，导致了性能严重下降，NGINX有一个默认配置，在一条连接上最多可以进行1000个请求。
```nginx
server {
   listen       10000 ssl http2;
   keepalive_requests 1000;
}
```
**这里的参数初步看起来很正常，对于http1.1 100并发下新建一批连接可以发送100*1000=100000个请求,所以TPS是正常的，但是，对于http2来说该参数的单位是stream，即一次请求+响应，也就是一批连接（对于http2无论多少并发只需要1条连接）仅可以发送1000个请求，是http1.1的1/100!!!!
在http1.1下，如果请求数是100000，那么第一批创建的100个连接就可以完成所有请求。
在http2下，如果请求数是100000，那么第一批创建的1个连接只能完成1000个请求，也就是说需要创建100批连接，这需要大量的CPU和时间**

{% blockquote%}
细心的同学可能会发现，《创建100批连接》这个形容有点怪，这是因为常用http client的连接池复用机制。
其原理是：当用户发起请求时：
1. 检查是连接池内否有符合要求的链接（复用就在这里发生），如果有就用该链接发起网络请求
2. 如果没有就创建一个链接发起请求。

对于http2，当连接池没有合适的链接时，会创建新的链接，在并发情况下，会创建一批链接而不是一个，这会表现为突然创建了100个链接， 然后将第一个链接放回连接池，剩下的99个链接直接被关闭。在nginx的默认配置下，这样的大批量创建链接并关闭会发生多次。这也是上文这个使用 http2压测创建了4000+次链接而不是100个的原因。
{% endblockquote %}

## 问题总结及解决方案
总而言之，问题就在于NGINX的keepalive_requests参数，这个参数的默认值是1000，对于http1.1来说，这个值是合理的，但是对于http2来说，这个值是不合理的，应该根据情况调整的大一些。
在这里，我将keepalive_requests的值调整为100000，然后重新压测，发现性能已经恢复正常了。
{% blockquote%}
一点个人感想：
NGINX是个很优秀的流量网关，但是由于扩展性和功能上的问题，往往会形成NGINX+API Gateway的模式，虽然也有一些基于nginx+lua的轻量解决方案，但是不够纯粹，性能也会受影响。
不妨试试[envoy](https://www.envoyproxy.io/)这个后起之秀吧，阿里基于envoy开发了[higress](https://higress.io/zh-cn/),可以把流量网关+API Gateway的功能都集成在一起，性能也很不错。
最后，让我们给俄罗斯开发者开源的NGINX致以崇高的敬意。
{% endblockquote %}
```bash
hey -c 100 -z 30s -m POST -d "1" -h2  https://localhost:9999/delay
```
```log
Summary:
  Total:	30.0032 secs
  Slowest:	0.5792 secs
  Fastest:	0.0012 secs
  Average:	0.0103 secs
  Requests/sec:	9725.2897
  
  New connection:	100

Response time histogram:
  0.059 [291576]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■

Latency distribution:
  10% in 0.0048 secs
  25% in 0.0068 secs
  50% in 0.0089 secs
  75% in 0.0116 secs
  90% in 0.0162 secs
  95% in 0.0212 secs
  99% in 0.0338 secs

Details (average, fastest, slowest):
  DNS+dialup:		0.0000 secs, 0.0000 secs, 0.0309 secs
  DNS-lookup:		0.0000 secs, 0.0000 secs, 0.0030 secs
  req write:		0.0000 secs, 0.0000 secs, 0.0025 secs
  resp wait:		0.0000 secs, 0.0012 secs, 0.5451 secs
  resp read:		0.0000 secs, 0.0000 secs, 0.0020 secs

Status code distribution:
  [200]	291790 responses
```

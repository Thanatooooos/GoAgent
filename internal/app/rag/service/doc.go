// Package service is the stable application-facing entry for RAG use cases.
//
// Implementations live in responsibility-focused subpackages (conversation, chat,
// sessionrecall, trace, longtermmemory). This root package keeps type aliases and
// constructor forwarding so bootstrap and HTTP adapters can keep importing
// ragservice without churn.
package service

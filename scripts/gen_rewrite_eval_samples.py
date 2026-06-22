#!/usr/bin/env python3
"""Generate testdata/rewrite_eval_samples.json from corpus and fixture queries."""

from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORPUS_PATH = ROOT / "testdata" / "retrieve_eval_corpus_samples.json"
RETRIEVE_FIXTURE_PATH = ROOT / "testdata" / "retrieve_eval_samples.json"
OUT_PATH = ROOT / "testdata" / "rewrite_eval_samples.json"

NEED_TRUE = True
NEED_FALSE = False
DEFAULT_MAX_SUBS = 4


def sample(name: str, query: str, tags: list[str], expect: dict, history: list | None = None) -> dict:
    item = {"name": name, "query": query, "tags": tags, "expect": expect}
    if history:
        item["history"] = history
    return item


def base_expect(**kwargs) -> dict:
    expect = {"needRetrieval": NEED_TRUE, "subQuestionCountMax": DEFAULT_MAX_SUBS}
    expect.update(kwargs)
    return expect


def skip_expect(**kwargs) -> dict:
    expect = {"needRetrieval": NEED_FALSE, "subQuestionCountMax": 2}
    expect.update(kwargs)
    return expect


# Per-sample expect overrides keyed by corpus/fixture name.
EXPECT_BY_NAME: dict[str, dict] = {
    "go_defer_lifo": {"mustKeepTerms": ["defer"]},
    "go_slice_growth_118": {"mustKeepTerms": ["slice"], "mustKeepAnyGroups": [["1.18", "扩容"]]},
    "go_map_hmap_structure": {"mustKeepTerms": ["hmap"]},
    "go_csp_model": {"mustKeepTerms": ["CSP"]},
    "go_channel_send_flow": {"mustKeepAnyGroups": [["channel", "Channel"]]},
    "go_select_random": {"mustKeepTerms": ["select"]},
    "go_context_with_cancel": {"mustKeepTerms": ["WithCancel", "context"]},
    "go_async_preemption": {"mustKeepTerms": ["SIGURG"], "mustKeepAnyGroups": [["异步抢占", "preemption"]]},
    "go_gmp_when_create_m": {"mustKeepAnyGroups": [["M", "GMP"], ["创建"]]},
    "go_goroutine_leak_causes": {"mustKeepAnyGroups": [["goroutine", "泄漏", "leak"]]},
    "go_sync_pool_usage": {"mustKeepTerms": ["sync.Pool", "Pool"]},
    "go_gc_stw_phases": {"mustKeepTerms": ["STW"], "mustKeepAnyGroups": [["GC", "垃圾回收"]]},
    "go_errgroup_vs_waitgroup": {"mustKeepTerms": ["errgroup", "WaitGroup"]},
    "metadata_file_go_md": {"mustKeepAnyGroups": [["Go.md", "Go"], ["slice", "扩容"]]},
    "redis_why_fast": {"mustContainAny": [["Redis", "redis"]]},
    "redis_memory_eviction": {"mustKeepAnyGroups": [["淘汰", "eviction"], ["内存"]]},
    "redis_sds_expansion": {"mustKeepTerms": ["SDS"]},
    "redis_unlink_not_del": {"mustKeepTerms": ["unlink", "del"]},
    "redis_bgsave_vs_save": {"mustKeepTerms": ["bgsave", "save"]},
    "redis_aof_rewrite": {"mustKeepTerms": ["AOF"]},
    "redis_replication_psync": {"mustKeepTerms": ["psync"]},
    "redis_cluster_slots": {"mustKeepAnyGroups": [["16384", "哈希槽", "slot"]]},
    "supplement_base62_base64": {"mustKeepAnyGroups": [["base62", "base64"]]},
    "supplement_redis_multi_exec": {"mustKeepAnyGroups": [["MULTI", "EXEC"]]},
    "supplement_int_vs_varchar_index": {"mustKeepAnyGroups": [["int", "varchar"], ["索引", "index"]]},
    "supplement_mysql_gtid_failover": {"mustKeepTerms": ["GTID"]},
    "linux_cpu_100_debug": {"mustKeepAnyGroups": [["CPU", "top"], ["100%"]]},
    "linux_find_large_files": {"mustKeepAnyGroups": [["find", "大文件"], ["100M"]]},
    "linux_listen_ports": {"mustKeepAnyGroups": [["端口", "listen", "ss", "netstat"]]},
    "linux_pprof_goroutine": {"mustKeepAnyGroups": [["pprof", "goroutine"]]},
    "linux_online_incident_playbook": {"mustKeepAnyGroups": [["排查", "异常", "服务", "诊断"]]},
    "network_http_https_diff": {"mustKeepAnyGroups": [["HTTP", "HTTPS"]]},
    "network_tls_client_hello": {"mustKeepAnyGroups": [["TLS", "Client Hello"]]},
	"network_http2_improvements": {"mustKeepAnyGroups": [["HTTP2", "HTTP/2", "HTTP1", "HTTP/1"]]},
    "network_time_wait_reason": {"mustKeepTerms": ["TIME_WAIT"]},
    "colloquial_redis_fast": {"mustContainAny": [["Redis", "redis"]]},
    "colloquial_go_map_unordered": {"mustContainAny": [["map", "Map"]]},
    "multi_go_map_structure_and_expand": {"mustKeepAnyGroups": [["map", "扩容"]]},
    "multi_go_gmp_and_netpoller": {"mustKeepAnyGroups": [["GMP", "调度"], ["netpoller", "多路复用", "IO"]]},
    "multi_go_leak_types_and_metrics": {"mustKeepAnyGroups": [["RSS", "heap"], ["内存", "泄漏"]]},
    "multi_redis_aof_and_rdb": {"mustKeepTerms": ["AOF", "RDB"]},
    "multi_redis_full_resync": {"mustKeepAnyGroups": [["全量", "同步"], ["主库", "从库"]]},
    "multi_linux_cpu_and_memory": {"mustKeepAnyGroups": [["CPU", "内存"], ["排查"]]},
    "multi_https_risks_and_fixes": {"mustKeepAnyGroups": [["HTTPS", "TLS"], ["窃听", "篡改"]]},
    "multi_redis_zset_skiplist": {"mustKeepAnyGroups": [["sorted set", "zset", "跳表", "skiplist"]]},
    "scenario_channel_send_blocked": {"mustKeepAnyGroups": [["channel", "阻塞"], ["发送"]]},
    "scenario_redis_memory_pressure": {"mustKeepAnyGroups": [["内存", "过期"], ["持久化", "AOF", "RDB"]]},
    "scenario_incident_cpu_then_logs": {"mustKeepAnyGroups": [["CPU", "top"], ["ERROR", "日志"]]},
    "scenario_https_upgrade": {"mustKeepAnyGroups": [["HTTPS", "TLS"], ["握手"]]},
    "scenario_goroutine_leak_pprof": {"mustKeepTerms": ["pprof"], "mustKeepAnyGroups": [["goroutine", "泄漏"]]},
    "scenario_raft_membership_change": {"mustKeepTerms": ["Raft"]},
    "scenario_slice_map_expand_confusion": {"mustKeepAnyGroups": [["slice", "map"], ["扩容"]]},
    "scenario_cancel_downstream_goroutines": {"mustKeepAnyGroups": [["context", "取消"], ["goroutine"]]},
    "scenario_redis_expire_vs_eviction": {"mustKeepAnyGroups": [["TTL", "过期"], ["淘汰", "eviction"]]},
    # retrieve_eval fixture extras
    "alias_vector_db_chinese": {"mustContainAny": [["向量数据库", "向量库", "vector"]]},
    "alias_rag_abbreviation": {"mustKeepTerms": ["RAG"]},
    "diagnosis_task_failure": {"mustKeepTerms": ["task_run_01"]},
    "diagnosis_trace_root_cause": {"mustKeepTerms": ["trace_bad_01"]},
    "multi_condition_doc_http_error": {"mustKeepTerms": ["doc_fail_01", "500"]},
    "multi_condition_pg_pool_timeout": {"mustContainAny": [["PostgreSQL", "postgres", "pg"]], "mustKeepAnyGroups": [["连接超时", "连接池"]]},
    "keyword_bm25_exact_match": {"mustKeepTerms": ["ef_construction", "HNSW"]},
    "keyword_error_code_lookup": {"mustKeepTerms": ["ERR_CONN_REFUSED"]},
    "metadata_file_name_lookup": {"mustKeepTerms": ["trace_handlers.go"]},
    "metadata_source_file_config": {"mustKeepTerms": ["application.yaml"]},
    "coreference_rag_applications": {"mustKeepTerms": ["RAG"], "mustNotStartWith": ["它", "这"]},
}


def corpus_rewrite_tags(corpus_tags: list[str]) -> list[str]:
    tags: list[str] = []
    mapping = {
        "colloquial": "colloquial",
        "diagnosis": "diagnosis",
        "multi_chunk": "multi_intent",
        "keyword": "keyword_preserve",
        "alias": "term_normalize",
        "metadata": "metadata",
        "rewrite": "rewrite",
        "semantic": "semantic",
    }
    for tag in corpus_tags:
        mapped = mapping.get(tag)
        if mapped and mapped not in tags:
            tags.append(mapped)
    return tags or ["semantic"]


def build_expect(name: str, corpus_tags: list[str]) -> dict:
    overrides = EXPECT_BY_NAME.get(name, {})
    expect = base_expect()
    if overrides:
        expect.update(overrides)
    # keyword_preserve samples should always have some term/group check
    if "keyword_preserve" in corpus_rewrite_tags(corpus_tags):
        has_check = any(k in expect for k in ("mustKeepTerms", "mustKeepAnyGroups", "mustContainAny"))
        if not has_check:
            expect["mustKeepAnyGroups"] = [[name.replace("_", " ")]]
    return expect


def load_corpus_samples() -> list[dict]:
    data = json.loads(CORPUS_PATH.read_text(encoding="utf-8"))
    samples = []
    for item in data["samples"]:
        name = item["name"]
        samples.append(
            sample(
                name,
                item["query"],
                corpus_rewrite_tags(item.get("tags", [])),
                build_expect(name, item.get("tags", [])),
            )
        )
    return samples


def load_fixture_extras(existing_names: set[str]) -> list[dict]:
    data = json.loads(RETRIEVE_FIXTURE_PATH.read_text(encoding="utf-8"))
    extras = []
    fixture_tags = {
        "alias_vector_db_chinese": ["term_normalize", "alias"],
        "alias_rag_abbreviation": ["colloquial", "alias"],
        "diagnosis_task_failure": ["constraint_id", "diagnosis"],
        "diagnosis_trace_root_cause": ["constraint_id", "diagnosis"],
        "diagnosis_latest_import_failure": ["diagnosis", "colloquial"],
        "multi_condition_doc_http_error": ["constraint_id", "multi_intent", "diagnosis"],
        "multi_condition_pg_pool_timeout": ["multi_intent", "term_normalize"],
        "keyword_bm25_exact_match": ["keyword_preserve"],
        "keyword_error_code_lookup": ["keyword_preserve", "diagnosis"],
        "metadata_file_name_lookup": ["metadata", "keyword_preserve"],
        "metadata_section_title_lookup": ["metadata"],
        "metadata_document_title": ["metadata"],
        "metadata_source_file_config": ["metadata", "keyword_preserve"],
        "coreference_rag_applications": ["coreference"],
        "coreference_document_chunk_count": ["coreference", "metadata"],
        "semantic_rag_concept": ["semantic"],
    }
    for item in data["samples"]:
        name = item["name"]
        if name in existing_names:
            continue
        tags = fixture_tags.get(name, ["semantic"])
        overrides = EXPECT_BY_NAME.get(name, {})
        if name == "diagnosis_latest_import_failure":
            overrides = {"mustKeepAnyGroups": [["导入", "失败"]]}
        if name == "metadata_section_title_lookup":
            overrides = {"mustKeepAnyGroups": [["概述", "章节"]]}
        if name == "metadata_document_title":
            overrides = {"mustKeepAnyGroups": [["部署指南"]]}
        if name == "coreference_document_chunk_count":
            overrides = {"mustContainAny": [["chunk", "文档"]], "mustNotStartWith": ["这个", "它"]}
        if name == "semantic_rag_concept":
            overrides = {"mustKeepTerms": ["RAG"]}
        expect = base_expect(**overrides)
        extras.append(sample(name, item["query"], tags, expect))
    return extras


def manual_extras(existing_names: set[str]) -> list[dict]:
    samples = []
    manual = [
        sample("alias_pg_connection", "pg 连接超时怎么处理", ["term_normalize", "alias"], base_expect(mustContainAny=[["PostgreSQL", "postgres", "pg"]])),
        sample("alias_es_health", "es 集群健康检查命令", ["term_normalize", "alias"], base_expect(mustContainAny=[["Elasticsearch", "elastic", "es"]])),
        sample("diagnosis_doc_failure", "doc_fail_01 为什么导入失败", ["constraint_id", "diagnosis"], base_expect(mustKeepTerms=["doc_fail_01"])),
        sample("coref_vector_db_apps", "它有哪些应用场景", ["coreference"], base_expect(
            mustContainAny=[["向量数据库", "vector"]],
            mustNotStartWith=["它", "这"],
        ), history=[
            {"role": "user", "content": "什么是向量数据库"},
            {"role": "assistant", "content": "向量数据库是一种专门用于存储和检索高维向量的数据库，常用于语义搜索和 RAG。"},
        ]),
        sample("coref_redis_persistence", "那持久化呢", ["coreference"], base_expect(
            mustContainAny=[["Redis", "持久化", "AOF", "RDB"]],
            mustNotStartWith=["那", "它"],
        ), history=[
            {"role": "user", "content": "Redis 为什么那么快"},
            {"role": "assistant", "content": "Redis 基于内存、单线程事件循环和高效数据结构。"},
        ]),
        sample("coref_go_slice_followup", "那扩容规则呢", ["coreference"], base_expect(
            mustContainAny=[["slice", "扩容"]],
            mustNotStartWith=["那", "它"],
        ), history=[
            {"role": "user", "content": "Go 1.18 以后 slice 怎么扩容"},
            {"role": "assistant", "content": "Go 1.18 之后 slice 扩容会同时考虑内存对齐和阈值策略。"},
        ]),
        sample("coref_redis_unlink_followup", "为啥不用 del", ["coreference", "keyword_preserve"], base_expect(
            mustKeepTerms=["del"],
            mustContainAny=[["unlink", "大 key", "Redis"]],
            mustNotStartWith=["为啥"],
        ), history=[
            {"role": "user", "content": "删除大 key 为什么用 unlink 不用 del"},
            {"role": "assistant", "content": "unlink 可以异步释放内存，避免阻塞主线程。"},
        ]),
    ]
    for item in manual:
        if item["name"] not in existing_names:
            samples.append(item)
            existing_names.add(item["name"])

    for name, query in [
        ("skip_hello", "你好"),
        ("skip_thanks", "谢谢"),
        ("skip_bye", "再见"),
        ("skip_who_are_you", "你是谁"),
        ("skip_calc", "17*23"),
        ("skip_ok", "好的"),
        ("skip_preference_chinese", "以后默认用中文回答"),
        ("skip_preference_english", "Please answer in English by default"),
        ("skip_remember_fact", "记住我喜欢简洁回答"),
    ]:
        if name not in existing_names:
            samples.append(sample(name, query, ["skip_retrieval"], skip_expect()))
            existing_names.add(name)

    return samples


def main() -> None:
    samples = load_corpus_samples()
    names = {s["name"] for s in samples}
    samples.extend(load_fixture_extras(names))
    names = {s["name"] for s in samples}
    samples.extend(manual_extras(names))

    # stable order: corpus order first, then extras alphabetically
    order = {s["name"]: i for i, s in enumerate(samples)}
    samples.sort(key=lambda s: order.get(s["name"], 9999))

    doc = {
        "meta": {
            "description": "Rewrite-only eval set: output quality checks without retrieval.",
            "version": "2",
            "generatedAt": "2026-06-14",
            "sources": [
                "testdata/retrieve_eval_corpus_samples.json",
                "testdata/retrieve_eval_samples.json",
                "manual skip/coreference cases",
            ],
        },
        "samples": samples,
    }
    OUT_PATH.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"{len(samples)} samples -> {OUT_PATH}")


if __name__ == "__main__":
    main()

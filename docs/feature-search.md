# Search

Hybrid search combining semantic understanding and keyword matching to find relevant knowledge across all your vaults.

## Overview

Know uses a hybrid search architecture that fuses two complementary retrieval strategies:

- **Vector search** (semantic similarity): Handles paraphrasing, synonyms, and conceptual queries. Uses embeddings to find documents that are semantically close to the query even when exact words differ.
- **BM25 fulltext search** (keyword matching): Handles exact names, technical terms, and specific keywords that semantic search might miss.

Results from both strategies are combined using **Reciprocal Rank Fusion (RRF)**, where each result's score is calculated as:

```
RRF_score = sum(1 / (k + rank_i))    # k typically 60
```

This approach is based on Anthropic's Contextual Retrieval research, which found that hybrid BM25 + embeddings + reranking reduces retrieval failures by 67%.

## How It Works

1. **Query execution**: The query runs against both the vector index and the BM25 fulltext index in parallel.
2. **Filtering**: Pre-filter narrows results by labels, document type, or folder path using indexes.
3. **Rank fusion**: RRF merges the two ranked result lists into a single ordering.
4. **Context assembly**: Top-20 results are assembled within a token budget. Full chunks (~800 tokens) are returned to the LLM, not truncated snippets.
5. **Contextual retrieval**: Document context is prepended to chunks before embedding, reducing top-20 retrieval failure by 35%.

## Usage

Search is available through the MCP `search_documents` tool and the agent chat's `kb_search` tool. Both use the same hybrid search pipeline and support `label`, `doc_type`, and `folder` filters.

Example agent prompts:

```
"Search for authentication patterns"
"Find documents about Kubernetes labeled 'infrastructure'"
"What do I know about the auth service?"
"Search the /docs/guides folder for deployment instructions"
```

## Reference

- **Search strategy**: Hybrid BM25 + vector with RRF fusion
- **Retrieval depth**: Top-20 results
- **Chunk size**: ~800 tokens (full chunks, not snippets)
- **Fusion constant**: k = 60
- **Filter options**: `label`, `doc_type`, `folder` (via MCP/agent tools)
- **Based on**: [Anthropic Contextual Retrieval](https://www.anthropic.com/news/contextual-retrieval) research

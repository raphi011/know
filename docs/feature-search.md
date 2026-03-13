# Search

Hybrid search combining semantic understanding and keyword matching to find relevant knowledge across all your vaults.

## Overview

Knowhow uses a hybrid search architecture that fuses two complementary retrieval strategies:

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

### CLI Search

```bash
# Simple search
knowhow search "authentication"

# Filter by labels
knowhow search "token refresh" --labels "work,auth-service"

# Filter by type
knowhow search "senior engineer" --type person

# Only verified knowledge
knowhow search "kubernetes" --verified
```

### Ask Questions (RAG)

The `ask` command performs a search, assembles the top results as context, and streams an LLM-generated answer.

```bash
# Free-form question (streams response token by token)
knowhow ask "What do I know about John Doe?"

# Ask about a service
knowhow ask "How does the auth service work?"

# Disable streaming for scripting/piping
knowhow ask "How does auth work?" --no-stream | head -5

# Use a template for structured output (non-streaming)
knowhow ask "John Doe" --template "Peer Review" -o review.md
knowhow ask "auth-service" --template "Service Summary"

# Filter context during ask
knowhow ask "What are John's responsibilities?" --labels "work" --type person
```

**Streaming behavior:**

- Default: Streams tokens in real-time for interactive use.
- Auto-disables when writing to file (`-o`), piping output, or using templates.
- Override with the `--no-stream` flag.

### MCP Tool

The `search_documents` MCP tool exposes the same hybrid search to AI agents. It searches across all accessible vaults and supports label, doc_type, and folder filters.

## Reference

- **Search strategy**: Hybrid BM25 + vector with RRF fusion
- **Retrieval depth**: Top-20 results
- **Chunk size**: ~800 tokens (full chunks, not snippets)
- **Fusion constant**: k = 60
- **Filter options**: `--labels`, `--type`, `--verified`, folder path
- **Based on**: [Anthropic Contextual Retrieval](https://www.anthropic.com/news/contextual-retrieval) research

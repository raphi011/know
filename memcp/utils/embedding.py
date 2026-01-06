"""Embedding and NLI utilities for memcp."""

import logging
import time
from functools import lru_cache

from sentence_transformers import SentenceTransformer, CrossEncoder

logger = logging.getLogger("memcp")

# Embedding model: Transforms text into 384-dimensional vectors where semantically
# similar texts cluster together in vector space. "all-MiniLM-L6-v2" is a lightweight
# model trained on 1B+ sentence pairs - good balance of speed vs quality.
embedder = SentenceTransformer('all-MiniLM-L6-v2')

# NLI (Natural Language Inference) model: Given two sentences, classifies their
# relationship as contradiction/entailment/neutral. Uses DeBERTa architecture
# fine-tuned on SNLI+MNLI datasets. CrossEncoder means both sentences are processed
# together (vs bi-encoder which encodes separately) - slower but more accurate.
nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')

# NLI output labels
NLI_LABELS = ['contradiction', 'entailment', 'neutral']


@lru_cache(maxsize=1000)
def _embed_cached(text: str) -> tuple[float, ...]:
    """Cached embedding - returns tuple for hashability."""
    return tuple(embedder.encode(text).tolist())


def embed(text: str) -> list[float]:
    """
    Convert text to dense vector representation. The model maps semantically
    similar texts to nearby points in 384-dim space. This enables "fuzzy" search -
    "canine" matches "dog" even though they share no characters.

    Uses LRU cache (1000 entries, ~1.5MB) to avoid redundant computations.
    """
    start = time.time()
    # Check cache stats before call
    cache_info = _embed_cached.cache_info()
    was_cached = cache_info.hits

    result = list(_embed_cached(text))

    # Determine if this was a cache hit
    new_cache_info = _embed_cached.cache_info()
    hit = new_cache_info.hits > was_cached

    duration_ms = (time.time() - start) * 1000
    logger.debug(f"[embed] {duration_ms:.1f}ms len={len(text)} cache={'HIT' if hit else 'MISS'}")

    return result


def get_embed_cache_stats() -> dict:
    """Get embedding cache statistics."""
    info = _embed_cached.cache_info()
    return {
        'hits': info.hits,
        'misses': info.misses,
        'size': info.currsize,
        'maxsize': info.maxsize,
        'hit_rate': info.hits / (info.hits + info.misses) if (info.hits + info.misses) > 0 else 0
    }


def check_contradiction(text1: str, text2: str) -> dict:
    """
    Use NLI to detect logical conflicts between statements. The model outputs
    logits for each class; we return both the winning label and raw scores
    (useful for thresholding - e.g., only flag if contradiction score > 0.8).
    """
    scores = nli_model.predict([(text1, text2)])
    label_idx = scores.argmax()
    return {
        'label': NLI_LABELS[label_idx],
        'scores': {NLI_LABELS[i]: float(scores[i]) for i in range(3)}
    }

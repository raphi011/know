"""Embedding and NLI utilities for memcp."""

import logging
import threading
import time
from functools import lru_cache

logger = logging.getLogger("memcp")

# Lazy-loaded models (loaded on first use, not at import time)
_embedder = None
_nli_model = None
_loading_lock = threading.Lock()

# NLI output labels
NLI_LABELS = ['contradiction', 'entailment', 'neutral']


def preload_models():
    """Start loading models in background thread. Call at startup for faster first query."""
    def _load():
        _get_embedder()
        _get_nli_model()
    thread = threading.Thread(target=_load, daemon=True)
    thread.start()
    return thread


def _get_embedder():
    """Lazy-load the embedding model (thread-safe)."""
    global _embedder
    if _embedder is None:
        with _loading_lock:
            if _embedder is None:  # Double-check after acquiring lock
                from sentence_transformers import SentenceTransformer
                logger.info("Loading embedding model (all-MiniLM-L6-v2)...")
                _embedder = SentenceTransformer('all-MiniLM-L6-v2')
                logger.info("Embedding model loaded")
    return _embedder


def _get_nli_model():
    """Lazy-load the NLI model (thread-safe)."""
    global _nli_model
    if _nli_model is None:
        with _loading_lock:
            if _nli_model is None:  # Double-check after acquiring lock
                from sentence_transformers import CrossEncoder
                logger.info("Loading NLI model (cross-encoder/nli-deberta-v3-base)...")
                _nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')
                logger.info("NLI model loaded")
    return _nli_model


@lru_cache(maxsize=1000)
def _embed_cached(text: str) -> tuple[float, ...]:
    """Cached embedding - returns tuple for hashability."""
    return tuple(_get_embedder().encode(text).tolist())


def embed(text: str) -> list[float]:
    """
    Convert text to dense vector representation. The model maps semantically
    similar texts to nearby points in 384-dim space. This enables "fuzzy" search -
    "canine" matches "dog" even though they share no characters.

    Uses LRU cache (1000 entries, ~1.5MB) to avoid redundant computations.
    Model is loaded lazily on first call.
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
    Model is loaded lazily on first call.
    """
    nli_model = _get_nli_model()
    scores = nli_model.predict([(text1, text2)])
    # scores is 2D: (batch_size, num_classes) - get first result
    scores_row = scores[0] if len(scores.shape) > 1 else scores
    label_idx = scores_row.argmax()
    return {
        'label': NLI_LABELS[label_idx],
        'scores': {NLI_LABELS[i]: float(scores_row[i]) for i in range(3)}
    }

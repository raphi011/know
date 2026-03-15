import Foundation

struct FuzzyMatchResult {
    let score: Int
    let matchedIndices: [Int]
}

/// Performs fuzzy matching of a query against a target string.
///
/// Characters must appear in order but not contiguously. Scores higher for
/// consecutive matches, word-boundary matches, and path-segment-start matches.
func fuzzyMatch(query: String, target: String) -> FuzzyMatchResult? {
    guard !query.isEmpty else { return nil }

    let queryChars = Array(query.lowercased())
    let targetChars = Array(target.lowercased())
    let originalChars = Array(target)

    guard queryChars.count <= targetChars.count else { return nil }

    var matchedIndices: [Int] = []
    var score = 0
    var queryIndex = 0
    var previousMatchIndex = -2 // sentinel for "no previous match"

    for targetIndex in targetChars.indices {
        guard queryIndex < queryChars.count else { break }

        if targetChars[targetIndex] == queryChars[queryIndex] {
            matchedIndices.append(targetIndex)

            // Consecutive match bonus
            if targetIndex == previousMatchIndex + 1 {
                score += 8
            }

            // Boundary bonus: match right after a separator
            if targetIndex == 0 {
                score += 10
            } else {
                let prev = originalChars[targetIndex - 1]
                if prev == "/" || prev == "-" || prev == "_" || prev == " " || prev == "." {
                    score += 8
                }
                // CamelCase boundary
                if prev.isLowercase && originalChars[targetIndex].isUppercase {
                    score += 6
                }
            }

            // Base score for any match
            score += 1

            previousMatchIndex = targetIndex
            queryIndex += 1
        }
    }

    // All query characters must be matched
    guard queryIndex == queryChars.count else { return nil }

    // Penalty for longer targets (prefer shorter paths)
    score -= targetChars.count / 10

    return FuzzyMatchResult(score: score, matchedIndices: matchedIndices)
}

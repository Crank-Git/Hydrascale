---
paths:
  - "docs/**/*.md"
  - "*.md"
  - ".claude/rules/*.md"
  - ".claude/skills/**/*.md"
  - "**/*.go"
---

# Writing standard — Simplified Technical English

Every specification file, issue body, comment, manual page, and **code comment** in this
project uses Simplified Technical English. A requirement that reads two ways gets built
two ways.

The project's controlled vocabulary is the `## Terms` table in `docs/specs/spec.md`. Read
it before you write a domain word. When you need a word that the table does not hold, add
a row to the table.

## Reproduce verbatim — never rewrite

- Anything a person said: an operator answer, a review comment, a dictated requirement.
  Quote it, then restate it in this standard underneath if it needs clarifying.
- Evidence: an error message, a log excerpt, console output, a test name, a stack trace,
  command output, a `file:line` reference.
- Code, configuration, commands, JSON, file paths, identifiers, and label names.
- A third-party product name and an API field name.

Rewriting evidence destroys it. Rewriting a quote misrepresents the person.

## The rules

### Sentences

1. A procedure sentence holds 20 words or fewer. A description sentence holds 25 or
   fewer. Count the words. Over the limit, split the sentence.
2. One instruction per sentence.
3. One topic per paragraph, and six sentences at most.
4. Put the condition first, then the action. Write "If the test fails, revert the
   commit."
5. Put a warning before the step it applies to, never after.

### Words

6. One word, one meaning, one part of speech.
7. One concept, one word. Never rotate a synonym for variety.
8. Keep a noun cluster to three words.
9. Use no metaphor, no idiom, and no slang. Name the mechanism instead. This project
   rejects "clobber", "stomp", "leak" as a verb, and "shield" as a noun.
10. Define an abbreviation once, on first use, in the Terms table.

### Grammar

11. Use the active voice. The reader must know who acts.
12. Use the present tense for behaviour and the imperative for an instruction.
13. Keep the articles. Write "the branch", not "branch".
14. Use no `-ing` form as a noun or as a heading.
15. Write positively. Keep a prohibition only when the wrong action is likely and costly.
16. Use a vertical list when a sentence would carry more than two conditions.

## Patterns

### A functional requirement

One testable statement, active voice, present tense, no conjunction.

```
Bad:  FR-access-3 — Users should be able to set ports and the daemon will apply them.
Good: FR-access-3 — A local rule carries a list of ports.
      FR-access-4 — The daemon applies a changed rule set within 2 seconds.
```

"should be able to" is not testable. "within a short time" is not a value.

### An acceptance criterion

Write the observable result, not the implementation.

```
Bad:  Access control works correctly.
Good: A namespace cannot reach another namespace when no local rule allows it.
      The daemon writes 11 rules into HYDRASCALE-FWD.
```

### An issue title

An imperative verb, one deliverable, 10 words or fewer.

```
Bad:  ACL stuff / firewall improvements (part 2)
Good: Write the local rule set into the HYDRASCALE-FWD chain
```

### A code comment

One sentence, one fact, active voice. Say why, not what. Code already states what it
does; a comment that repeats the code is noise.

```go
// Bad:
// increment the counter
n += 1

// Good:
// tailscaled replaces resolv.conf with a rename, so a bind mount on the file is not
// enough. The overlay must cover the whole directory.
```

A doc comment opens with one sentence that states the result, then the parameters, then
the failure modes.

```go
// Good:
// Compile returns the iptables arguments that the rule set requires.
// Compile is pure: the same rule set and the same node addresses always produce the
// same output. Compile returns an error when a rule names an unknown tailnet.
```

A test name is a sentence: `rejects a rule where from equals to`, not `TestRule2`.

Keep a marker keyword — `TODO`, `FIXME`, `HACK` — because tooling matches on it. Write
the body to this standard and name the issue:
`// TODO(#412): Replace the fixed 2-second interval with the configured value.`

## Check before you write the file

- [ ] No sentence is longer than 25 words. No instruction is longer than 20.
- [ ] Every step holds one instruction.
- [ ] Every condition comes before its action. Every warning comes before its step.
- [ ] Every domain word is in the Terms table, with one meaning.
- [ ] One concept, one word, throughout the document.
- [ ] No noun cluster is longer than three words.
- [ ] No metaphor, no idiom, no undefined abbreviation.
- [ ] Active voice, present tense, articles present, no `-ing` noun or heading.
- [ ] A list carries anything with more than two conditions.
- [ ] Every quote, error message, path, and identifier is verbatim.
- [ ] A code comment states the reason. A doc comment opens with the result. A test name
      reads as one behaviour.

#!/usr/bin/env python3
"""Test the inline markdown conversion of render-spec.py.

Run it with `python3 docs/specs/render-spec-test.py`. It prints the count of the
tests that pass, and it exits 1 on the first failure.

The module name holds a hyphen, so importlib loads the file by path.
"""

import importlib.util
import pathlib
import sys

_SCRIPT = pathlib.Path(__file__).with_name("render-spec.py")
_spec = importlib.util.spec_from_file_location("render_spec", _SCRIPT)
render_spec = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(render_spec)

inline = render_spec.inline

TESTS = []


def test(fn):
    TESTS.append(fn)
    return fn


@test
def a_line_with_three_wildcard_code_spans_keeps_every_wildcard():
    """This is the example of issue #383. The emphasis substitution paired the
    asterisks of separate code spans, so two wildcards were absent from the page."""
    got = inline(
        "The wildcard `*` names every source, therefore the square "
        "from `*` to `*` accepts a click."
    )
    assert got == (
        "The wildcard <code>*</code> names every source, therefore the square "
        "from <code>*</code> to <code>*</code> accepts a click."
    ), got


@test
def an_asterisk_inside_a_code_span_starts_no_emphasis():
    got = inline("Write `*` and then *emphasis* holds the rest.")
    assert got == "Write <code>*</code> and then <em>emphasis</em> holds the rest.", got


@test
def emphasis_outside_a_code_span_still_renders():
    assert inline("*emphasis*") == "<em>emphasis</em>"


@test
def strong_outside_a_code_span_still_renders():
    assert inline("**strong**") == "<strong>strong</strong>"


@test
def a_double_asterisk_inside_a_code_span_starts_no_strong():
    got = inline("Compare `**` with `**` again.")
    assert got == "Compare <code>**</code> with <code>**</code> again.", got


@test
def a_link_that_holds_a_code_span_keeps_both():
    got = inline("Read [the `spec` page](spec.html).")
    assert got == 'Read <a href="spec.html">the <code>spec</code> page</a>.', got


@test
def a_code_span_escapes_html():
    assert inline("`<b>`") == "<code>&lt;b&gt;</code>"


def main():
    for fn in TESTS:
        try:
            fn()
        except AssertionError as err:
            print("FAIL %s\n%s" % (fn.__name__, err))
            return 1
    print("ok %d tests" % len(TESTS))
    return 0


if __name__ == "__main__":
    sys.exit(main())

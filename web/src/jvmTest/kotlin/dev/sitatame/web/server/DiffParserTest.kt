package dev.sitatame.web.server

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Sanity checks for the unified-diff parser. Full parity against the Go-side
 * `internal/gitx` parser is out of scope for Phase 1 step 1; the Web view only
 * needs file headers, hunks, and ± counts.
 */
class DiffParserTest {

    @Test
    fun `single hunk produces lines with correct gutters`() {
        val diff = """
            diff --git a/foo.go b/foo.go
            index aaaaaaa..bbbbbbb 100644
            --- a/foo.go
            +++ b/foo.go
            @@ -1,3 +1,4 @@
             package foo
            -import "fmt"
            +import (
            +	"context"
            +	"fmt"
            +)
        """.trimIndent()

        val files = DiffParser.parse(diff)
        assertEquals(1, files.size)
        val f = files[0]
        assertEquals("foo.go", f.path)
        assertEquals("M", f.status)
        assertEquals(4, f.adds)
        assertEquals(1, f.dels)
        assertEquals(1, f.hunks.size)
        val hunk = f.hunks[0]
        // First context line "package foo" is line 1 on both sides.
        assertEquals(1, hunk.lines[0].baseLine)
        assertEquals(1, hunk.lines[0].headLine)
        // Deleted "import \"fmt\"" is base line 2 only.
        assertEquals(2, hunk.lines[1].baseLine)
        assertNull(hunk.lines[1].headLine)
        assertEquals("-", hunk.lines[1].prefix)
        // First added line "import (" is head line 2 only.
        assertNull(hunk.lines[2].baseLine)
        assertEquals(2, hunk.lines[2].headLine)
        assertEquals("+", hunk.lines[2].prefix)
    }

    @Test
    fun `new file is detected as A`() {
        val diff = """
            diff --git a/new.go b/new.go
            new file mode 100644
            index 0000000..abcdef0
            --- /dev/null
            +++ b/new.go
            @@ -0,0 +1,2 @@
            +package new
            +
        """.trimIndent()

        val files = DiffParser.parse(diff)
        assertEquals(1, files.size)
        assertEquals("A", files[0].status)
        assertEquals(2, files[0].adds)
        assertEquals(0, files[0].dels)
    }

    @Test
    fun `deleted file uses prePath and is marked D`() {
        val diff = """
            diff --git a/old.go b/old.go
            deleted file mode 100644
            index abcdef0..0000000
            --- a/old.go
            +++ /dev/null
            @@ -1,2 +0,0 @@
            -package old
            -
        """.trimIndent()

        val files = DiffParser.parse(diff)
        assertEquals(1, files.size)
        assertEquals("D", files[0].status)
        assertEquals("old.go", files[0].path)
        assertEquals(2, files[0].dels)
    }

    @Test
    fun `rename captures renameFrom and renameTo`() {
        val diff = """
            diff --git a/old/path.go b/new/path.go
            similarity index 100%
            rename from old/path.go
            rename to new/path.go
        """.trimIndent()

        val files = DiffParser.parse(diff)
        assertEquals(1, files.size)
        val f = files[0]
        assertEquals("R", f.status)
        assertEquals("old/path.go", f.renameFrom)
        assertEquals("new/path.go", f.renameTo)
        assertEquals(0, f.hunks.size) // no body for a pure rename
    }

    @Test
    fun `multiple files emit separate entries`() {
        val diff = """
            diff --git a/a.go b/a.go
            --- a/a.go
            +++ b/a.go
            @@ -1 +1 @@
            -old
            +new
            diff --git a/b.go b/b.go
            --- a/b.go
            +++ b/b.go
            @@ -1 +1,2 @@
             keep
            +added
        """.trimIndent()

        val files = DiffParser.parse(diff)
        assertEquals(2, files.size)
        assertEquals("a.go", files[0].path)
        assertEquals(1, files[0].adds)
        assertEquals(1, files[0].dels)
        assertEquals("b.go", files[1].path)
        assertEquals(1, files[1].adds)
    }

    @Test
    fun `empty input returns empty list`() {
        val files = DiffParser.parse("")
        assertTrue(files.isEmpty())
    }

    @Test
    fun `hunk header tail is preserved`() {
        val diff = """
            diff --git a/foo.go b/foo.go
            --- a/foo.go
            +++ b/foo.go
            @@ -10,3 +12,3 @@ func Foo() {
             	a := 1
            -	b := 2
            +	b := 3
        """.trimIndent()

        val files = DiffParser.parse(diff)
        val hunk = files[0].hunks[0]
        assertNotNull(hunk)
        assertEquals("func Foo() {", hunk.header)
        assertEquals(10, hunk.baseStart)
        assertEquals(3, hunk.baseLines)
        assertEquals(12, hunk.headStart)
        assertEquals(3, hunk.headLines)
    }
}

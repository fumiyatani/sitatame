package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * Full-screen overlay modal for composing a comment.
 *
 * The modal is rendered in a [Box] with a semi-transparent scrim so the diff
 * pane is visible underneath.  A centred [Surface] holds the form.
 *
 * [target] describes what the comment will be anchored to and drives the
 * header text.  [title] overrides the default header derived from [target.label()];
 * pass a custom string (e.g. "Reply to: Line 42 · foo.kt") for reply mode.
 * [onSubmit] receives the non-blank body when the user clicks "Submit";
 * [onCancel] closes the modal without posting.
 */
@Composable
fun CommentModal(
    target: CommentTarget,
    onSubmit: (body: String) -> Unit,
    onCancel: () -> Unit,
    title: String = target.label(),
) {
    var body by remember { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }

    // Dim backdrop
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.55f)),
        contentAlignment = Alignment.Center,
    ) {
        Surface(
            modifier = Modifier
                .padding(horizontal = 48.dp, vertical = 32.dp)
                .shadow(8.dp, RoundedCornerShape(8.dp)),
            color = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(8.dp),
        ) {
            Column(modifier = Modifier.padding(20.dp)) {
                // Header
                Text(
                    text = title,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                )

                Spacer(modifier = Modifier.padding(top = 12.dp))

                // Textarea
                val colors = LocalSitatameColors.current
                Surface(
                    color = MaterialTheme.colorScheme.background,
                    shape = RoundedCornerShape(4.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    BasicTextField(
                        value = body,
                        onValueChange = { body = it },
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 100.dp, max = 300.dp)
                            .padding(10.dp)
                            .focusRequester(focusRequester),
                        textStyle = LocalTextStyle.current.copy(
                            color = MaterialTheme.colorScheme.onBackground,
                            fontFamily = FontFamily.Monospace,
                            fontSize = 13.sp,
                        ),
                        cursorBrush = SolidColor(MaterialTheme.colorScheme.primary),
                        decorationBox = { inner ->
                            Box {
                                if (body.isEmpty()) {
                                    Text(
                                        text = "Leave a comment…",
                                        color = colors.mutedText,
                                        fontFamily = FontFamily.Monospace,
                                        fontSize = 13.sp,
                                    )
                                }
                                inner()
                            }
                        },
                    )
                }

                Spacer(modifier = Modifier.padding(top = 12.dp))

                // Buttons
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    TextButton(onClick = onCancel) {
                        Text("Cancel", color = MaterialTheme.colorScheme.onSurface)
                    }
                    Spacer(Modifier.width(8.dp))
                    Button(
                        onClick = { if (body.isNotBlank()) onSubmit(body.trim()) },
                        enabled = body.isNotBlank(),
                        colors = ButtonDefaults.buttonColors(
                            containerColor = MaterialTheme.colorScheme.primary,
                            contentColor = MaterialTheme.colorScheme.onPrimary,
                            disabledContainerColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.38f),
                            disabledContentColor = MaterialTheme.colorScheme.onPrimary.copy(alpha = 0.38f),
                        ),
                    ) {
                        Text("Submit")
                    }
                }
            }
        }
    }

    // Request focus so the user can type immediately
    LaunchedEffect(Unit) {
        try {
            focusRequester.requestFocus()
        } catch (_: Exception) {
            // FocusRequester throws if the node is not yet composed; harmless.
        }
    }
}

/**
 * Variant of [CommentModal] pre-populated with an existing text (for editing
 * the review comment, B9).
 */
@Composable
fun EditTextModal(
    title: String,
    initialText: String,
    onSubmit: (text: String) -> Unit,
    onCancel: () -> Unit,
) {
    var body by remember(initialText) { mutableStateOf(initialText) }
    val focusRequester = remember { FocusRequester() }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.55f)),
        contentAlignment = Alignment.Center,
    ) {
        Surface(
            modifier = Modifier
                .padding(horizontal = 48.dp, vertical = 32.dp)
                .shadow(8.dp, RoundedCornerShape(8.dp)),
            color = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(8.dp),
        ) {
            Column(modifier = Modifier.padding(20.dp)) {
                Text(
                    text = title,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                )

                Spacer(modifier = Modifier.padding(top = 12.dp))

                val colors = LocalSitatameColors.current
                Surface(
                    color = MaterialTheme.colorScheme.background,
                    shape = RoundedCornerShape(4.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    BasicTextField(
                        value = body,
                        onValueChange = { body = it },
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 120.dp, max = 400.dp)
                            .padding(10.dp)
                            .focusRequester(focusRequester),
                        textStyle = LocalTextStyle.current.copy(
                            color = MaterialTheme.colorScheme.onBackground,
                            fontFamily = FontFamily.Monospace,
                            fontSize = 13.sp,
                        ),
                        cursorBrush = SolidColor(MaterialTheme.colorScheme.primary),
                        decorationBox = { inner ->
                            Box {
                                if (body.isEmpty()) {
                                    Text(
                                        text = "Write review summary…",
                                        color = colors.mutedText,
                                        fontFamily = FontFamily.Monospace,
                                        fontSize = 13.sp,
                                    )
                                }
                                inner()
                            }
                        },
                    )
                }

                Spacer(modifier = Modifier.padding(top = 12.dp))

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    TextButton(onClick = onCancel) {
                        Text("Cancel", color = MaterialTheme.colorScheme.onSurface)
                    }
                    Spacer(Modifier.width(8.dp))
                    Button(
                        onClick = { onSubmit(body) },
                        colors = ButtonDefaults.buttonColors(
                            containerColor = MaterialTheme.colorScheme.primary,
                            contentColor = MaterialTheme.colorScheme.onPrimary,
                        ),
                    ) {
                        Text("Save")
                    }
                }
            }
        }
    }

    LaunchedEffect(Unit) {
        try {
            focusRequester.requestFocus()
        } catch (_: Exception) { }
    }
}

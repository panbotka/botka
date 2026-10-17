/**
 * Copy text to the clipboard. Falls back to a hidden textarea plus
 * `execCommand('copy')` where the async Clipboard API is unavailable or
 * rejects (insecure origin, older WebViews).
 */
export async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
}

/**
 * Bridge from chat chrome outside ChatView (the mobile header "⋯" menu) to the
 * slash-command actions ChatView owns. Mirrors the existing
 * `botka:open-search` window-event pattern so ChatPage needs no ref into
 * ChatView.
 */
export const CHAT_COMMAND_EVENT = 'botka:chat-command'

export interface ChatCommandDetail {
  /** Thread the command targets; a ChatView showing another thread ignores it. */
  threadId: number
  command: '/find' | '/clear'
  /**
   * For `/clear`: the caller already asked the user to confirm, so skip the
   * "type /clear again" step that the typed slash command requires.
   */
  confirmed?: boolean
}

export function dispatchChatCommand(detail: ChatCommandDetail): void {
  window.dispatchEvent(new CustomEvent<ChatCommandDetail>(CHAT_COMMAND_EVENT, { detail }))
}

import { useEffect, useRef, useCallback } from 'react'
import type { Message } from '../types'

const CHANNEL_NAME = 'botka-chat-sync'

type SyncEvent =
  | { type: 'new-message'; threadId: number; message: Message }
  | { type: 'thread-updated'; threadId: number }

interface UseChatSyncOptions {
  threadId: number | null
  onNewMessage: (message: Message) => void
  onThreadUpdated: () => void
}

/**
 * Syncs chat messages across browser tabs using BroadcastChannel.
 * Broadcasts user messages immediately and signals thread updates
 * on stream completion so other tabs can refetch.
 */
export function useChatSync({ threadId, onNewMessage, onThreadUpdated }: UseChatSyncOptions) {
  const channelRef = useRef<BroadcastChannel | null>(null)

  useEffect(() => {
    const channel = new BroadcastChannel(CHANNEL_NAME)
    channelRef.current = channel

    channel.onmessage = (event: MessageEvent<SyncEvent>) => {
      const data = event.data
      if (!threadId) return

      if (data.type === 'new-message' && data.threadId === threadId) {
        onNewMessage(data.message)
      } else if (data.type === 'thread-updated' && data.threadId === threadId) {
        onThreadUpdated()
      }
    }

    return () => {
      channel.close()
      channelRef.current = null
    }
  }, [threadId, onNewMessage, onThreadUpdated])

  const broadcastNewMessage = useCallback((tid: number, message: Message) => {
    channelRef.current?.postMessage({
      type: 'new-message',
      threadId: tid,
      message,
    } satisfies SyncEvent)
  }, [])

  const broadcastThreadUpdated = useCallback((tid: number) => {
    channelRef.current?.postMessage({
      type: 'thread-updated',
      threadId: tid,
    } satisfies SyncEvent)
  }, [])

  return { broadcastNewMessage, broadcastThreadUpdated }
}

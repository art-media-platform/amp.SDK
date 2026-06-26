/**
 * useForumsApi — the single write path for the forums SPA.  Every mutation is an
 * app-verb invoke (amp://~/forums/{verb}); the ampd custodian authors the durable
 * write, recording the invoking member as the content author.  Members hold
 * ReadOnly on forum channels, so these verbs are the only way to write.
 */

import { useCallback } from 'react';
import { useAmpMutation, useAmpAuth } from '@art-media-platform/web';
import type { TxOp } from '@art-media-platform/web';
import {
  BOARD, PROFILES,
  ATTR_TOPIC, ATTR_POST, ATTR_PROFILE, ATTR_READSTATE, ATTR_SUBSCRIPTION,
  VERB, PostStatus,
} from '../forums-attrs';

export function useForumsApi() {
  const { invoke, loading, error } = useAmpMutation();
  const { member } = useAmpAuth();

  // The server mints the topic node; the caller re-queries the board afterward.
  const createTopic = useCallback((title: string, bodyHTML: string, forumID?: string) => {
    const ops: TxOp[] = [
      { Kind: 'upsert', Channel: BOARD, Attr: ATTR_TOPIC, Value: { Title: title, ...(forumID ? { Forum: forumID } : {}) } },
      { Kind: 'upsert', Channel: BOARD, Attr: ATTR_POST, Value: { BodyHTML: bodyHTML, BodySource: bodyHTML, Status: PostStatus.Live } },
    ];
    return invoke(VERB.topic, ops);
  }, [invoke]);

  const reply = useCallback((topicID: string, bodyHTML: string) => {
    const ops: TxOp[] = [
      { Kind: 'upsert', Channel: topicID, Attr: ATTR_POST, Value: { BodyHTML: bodyHTML, BodySource: bodyHTML, Status: PostStatus.Live } },
    ];
    return invoke(VERB.post, ops);
  }, [invoke]);

  const moderate = useCallback((topicID: string, postID: string, status: number, bodyHTML?: string) => {
    const ops: TxOp[] = [
      {
        Kind: 'upsert', Channel: topicID, Attr: ATTR_POST, ItemID: postID,
        Value: { Status: status, ...(bodyHTML !== undefined ? { BodyHTML: bodyHTML, BodySource: bodyHTML } : {}) },
      },
    ];
    return invoke(VERB.moderate, ops);
  }, [invoke]);

  const subscribe = useCallback((topicID: string, frequency: number) => {
    const ops: TxOp[] = [
      { Kind: 'upsert', Channel: topicID, Attr: ATTR_SUBSCRIPTION, Value: { Frequency: frequency } },
    ];
    return invoke(VERB.subscribe, ops);
  }, [invoke]);

  // Self-edit only: the server keys the write by the caller, ignoring ItemID.
  const saveProfile = useCallback((displayName: string, signatureHTML: string) => {
    if (!member) throw new Error('login required to edit a profile');
    const ops: TxOp[] = [
      { Kind: 'upsert', Channel: PROFILES, Attr: ATTR_PROFILE, ItemID: member.ID, Value: { DisplayName: displayName, SignatureHTML: signatureHTML } },
    ];
    return invoke(VERB.profile, ops);
  }, [invoke, member]);

  const markRead = useCallback((topicID: string, lastReadEditID: string) => {
    if (!member) return Promise.resolve([]);
    // The topic rides the Channel (NodeID), like post/view; the server keys the
    // read-state by the caller and stores it on the member's own node.
    const ops: TxOp[] = [
      { Kind: 'upsert', Channel: topicID, Attr: ATTR_READSTATE, Value: { LastReadEdit: lastReadEditID } },
    ];
    return invoke(VERB.read, ops);
  }, [invoke, member]);

  const recordView = useCallback((topicID: string) => {
    const ops: TxOp[] = [{ Kind: 'upsert', Channel: topicID, Attr: ATTR_TOPIC, Value: {} }];
    return invoke(VERB.view, ops);
  }, [invoke]);

  return { createTopic, reply, moderate, subscribe, saveProfile, markRead, recordView, loading, error };
}

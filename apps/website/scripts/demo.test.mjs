import assert from 'node:assert/strict';
import test from 'node:test';
import { createPlayback, createConversation, filterNotifications } from '../public/demo-playback.js';

function walkthrough() {
  let clock = 0;
  let nextId = 0;
  const timers = new Map();
  const seen = [];
  let state;
  const playback = createPlayback({
    frames: [
      { screen: 'welcome', duration: 100 },
      { screen: 'feed', duration: 200 },
      { screen: 'comments', duration: 100 },
      { screen: 'messages', duration: 100 },
    ],
    render: (frame) => seen.push(frame.screen),
    status: (value) => { state = value; },
    now: () => clock,
    schedule: (callback, delay) => {
      const id = ++nextId;
      timers.set(id, { at: clock + delay, callback });
      return id;
    },
    cancel: (id) => timers.delete(id),
  });
  function advance(ms) {
    const end = clock + ms;
    let next;
    while ((next = [...timers.entries()].sort((a, b) => a[1].at - b[1].at)[0]) && next[1].at <= end) {
      clock = next[1].at;
      timers.delete(next[0]);
      next[1].callback();
    }
    clock = end;
  }
  return { playback, advance, seen, timers, get state() { return state; } };
}

test('entering the viewport plays the four screens once and leaves the final screen visible', () => {
  const run = walkthrough();
  run.playback.setVisible(false);
  run.advance(1000);
  assert.deepEqual(run.seen, []);
  run.playback.setVisible(true);
  run.advance(500);
  assert.deepEqual(run.seen, ['feed', 'comments', 'messages']);
  assert.equal(run.state.done, true);
  assert.equal(run.timers.size, 0);
  run.playback.setVisible(false);
  run.playback.setVisible(true);
  run.advance(1000);
  assert.equal(run.seen.length, 3, 'Returning to the section must not replay it');
});

test('leaving the viewport or hiding the page suspends time, including a partially viewed frame', () => {
  const run = walkthrough();
  run.playback.setVisible(true);
  run.advance(65);
  run.playback.setVisible(false);
  assert.equal(run.timers.size, 0);
  run.advance(5000);
  assert.deepEqual(run.seen, []);
  run.playback.setVisible(true);
  run.advance(34);
  assert.deepEqual(run.seen, []);
  run.advance(1);
  assert.deepEqual(run.seen, ['feed']);
});

test('repeated visibility notifications do not restart or multiply the frame timer', () => {
  const run = walkthrough();
  run.playback.setVisible(true);
  run.advance(70);
  run.playback.setVisible(true);
  run.playback.setVisible(true);
  assert.equal(run.timers.size, 1);
  run.advance(30);
  assert.deepEqual(run.seen, ['feed']);
});

test('manual interaction pauses permanently until the user explicitly plays again', () => {
  const run = walkthrough();
  run.playback.pause();
  run.playback.setVisible(true);
  run.advance(1000);
  assert.deepEqual(run.seen, []);
  run.playback.play();
  run.advance(140);
  run.playback.pause();
  run.playback.setVisible(false);
  run.playback.setVisible(true);
  run.advance(1000);
  assert.deepEqual(run.seen, ['feed']);
  run.playback.play();
  run.advance(160);
  assert.deepEqual(run.seen, ['feed', 'comments']);
});

test('replay starts at welcome with a full duration and a single timer, even mid-playback', () => {
  const run = walkthrough();
  run.playback.setVisible(true);
  run.advance(320);
  run.playback.replay();
  assert.equal(run.seen.at(-1), 'welcome');
  assert.equal(run.state.done, false);
  assert.equal(run.timers.size, 1);
  run.advance(99);
  assert.equal(run.seen.at(-1), 'welcome');
  run.advance(1);
  assert.equal(run.seen.at(-1), 'feed');
  run.advance(400);
  assert.equal(run.state.done, true);
  run.playback.replay();
  assert.equal(run.seen.at(-1), 'welcome');
  assert.equal(run.state.playing, true);
});

test('reduced motion prevents automatic playback and replay resets to a static welcome', () => {
  const run = walkthrough();
  run.playback.setReduced(true);
  run.playback.setVisible(true);
  run.playback.play();
  run.advance(1000);
  assert.deepEqual(run.seen, []);
  assert.equal(run.timers.size, 0);
  run.playback.replay();
  assert.deepEqual(run.seen, ['welcome']);
  assert.equal(run.timers.size, 0);
  run.playback.setReduced(false);
  run.advance(1000);
  assert.deepEqual(run.seen, ['welcome'], 'Changing a preference must not unexpectedly resume');
});

test('enabling reduced motion during playback cancels its pending action', () => {
  const run = walkthrough();
  run.playback.setVisible(true);
  run.advance(150);
  run.playback.setReduced(true);
  run.advance(1000);
  assert.deepEqual(run.seen, ['feed']);
  assert.equal(run.timers.size, 0);
  run.playback.setReduced(false);
  run.advance(1000);
  assert.deepEqual(run.seen, ['feed']);
});

test('contact requires a selected identity and accepts exactly one first text', () => {
  const contact = createConversation();
  assert.equal(contact.send('你好'), false);
  assert.equal(contact.selectIdentity('南风'), true);
  assert.equal(contact.send('   '), false);
  assert.equal(contact.send('你好，周六可以一起吗？'), true);
  assert.equal(contact.waiting, true);
  assert.equal(contact.locked, true);
  assert.equal(contact.send('第二条不能提前发送'), false);
  assert.equal(contact.selectIdentity('另一个身份'), false);
  assert.equal(contact.messages.length, 1);
  assert.equal(contact.identity, '南风');
});

test('a simulated peer reply unlocks follow-up messages without changing the identity', () => {
  const contact = createConversation();
  assert.equal(contact.receive('不能凭空回复尚未发起的会话'), false);
  contact.selectIdentity('南风');
  contact.send('你好');
  assert.equal(contact.receive('好呀，九点见。'), true);
  assert.equal(contact.waiting, false);
  assert.equal(contact.send('九点见！'), true);
  assert.deepEqual(contact.messages.map((message) => message.outgoing), [true, false, true]);
  assert.equal(contact.identity, '南风');
});

test('conversation snapshots cannot mutate the stored local dialogue', () => {
  const contact = createConversation();
  contact.selectIdentity('南风'); contact.send('<img src=x>');
  const copy = contact.messages; copy[0].text = 'changed'; copy.push({text:'extra'});
  assert.equal(contact.messages.length, 1);
  assert.equal(contact.messages[0].text, '<img src=x>');
});

test('notification categories compose with unread filtering and retain chronological order', () => {
  const entries = [
    { id:'r', category:'reply', unread:true },
    { id:'l', category:'like', unread:true },
    { id:'s1', category:'system', unread:false },
    { id:'s2', category:'system', unread:true },
  ];
  assert.deepEqual(filterNotifications(entries).map(n => n.id), ['r','l','s1','s2']);
  assert.deepEqual(filterNotifications(entries,'reply',false).map(n => n.id), ['r']);
  assert.deepEqual(filterNotifications(entries,'like',true).map(n => n.id), ['l']);
  assert.deepEqual(filterNotifications(entries,'system',false).map(n => n.id), ['s1','s2']);
  assert.deepEqual(filterNotifications(entries,'system',true).map(n => n.id), ['s2']);
  assert.deepEqual(filterNotifications(entries,'all',true).map(n => n.id), ['r','l','s2']);
  assert.equal(entries.length,4,'Filtering must not remove stored notifications');
  entries[0].unread=false;
  assert.deepEqual(filterNotifications(entries,'reply',true),[],'Opening an unread reply removes it from unread results');
  assert.equal(filterNotifications(entries,'reply',false).length,1,'Read items remain in their category');
});

test('after the first fixed reply, repeated user sends do not return to first-contact waiting', () => {
  const contact=createConversation();contact.selectIdentity('南风');
  contact.send('第一条'); assert.equal(contact.waiting,true);
  contact.receive('你好，很高兴收到你的消息。');
  for(const text of ['第二条','第三条','第四条']) { assert.equal(contact.send(text),true); assert.equal(contact.waiting,false); }
  assert.equal(contact.messages.filter(message=>!message.outgoing).length,1);
});

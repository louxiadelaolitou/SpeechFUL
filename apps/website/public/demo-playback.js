// A single finite walkthrough. Visibility pauses preserve the remaining frame time.
export function createPlayback({
  frames, render, status,
  schedule = setTimeout, cancel = clearTimeout, now = () => performance.now(),
}) {
  let index = 0;
  let timer = null;
  let deadline = 0;
  let remaining = frames[0].duration;
  let wanted = false;
  let visible = false;
  let reduced = false;
  let started = false;
  let done = false;
  function stopTimer() {
    if (timer === null) return;
    remaining = Math.max(0, deadline - now());
    cancel(timer);
    timer = null;
  }
  function sync() {
    const playing = wanted && visible && !reduced && !done;
    if (!playing) stopTimer();
    status({ playing, wanted, done, index });
    if (!playing || timer !== null) return;
    deadline = now() + remaining;
    timer = schedule(() => {
      timer = null;
      if (index < frames.length - 1) {
        index += 1;
        remaining = frames[index].duration;
        render(frames[index]);
      }
      else { done = true; wanted = false; }
      sync();
    }, remaining);
  }
  return {
    setVisible(value) {
      visible = value;
      if (visible && !started && !reduced) { started = true; wanted = true; }
      sync();
    },
    setReduced(value) { reduced = value; if (value) { wanted = false; started = true; } sync(); },
    pause() { wanted = false; started = true; sync(); },
    play() { started = true; wanted = true; sync(); },
    replay() {
      stopTimer();
      index = 0;
      remaining = frames[0].duration;
      done = false;
      started = true;
      wanted = !reduced;
      render(frames[0]);
      sync();
    },
  };
}

// Local-only projection of the app's first-contact rule. This never represents
// authentication, a server acceptance, durable delivery, or a read receipt.
export function createConversation() {
  let identity = null;
  let firstSent = false;
  let replied = false;
  const messages = [];
  return {
    get identity() { return identity; },
    get waiting() { return firstSent && !replied; },
    get locked() { return firstSent; },
    get messages() { return messages.map((entry) => ({ ...entry })); },
    selectIdentity(value) {
      if (firstSent || !value) return false;
      identity = value;
      return true;
    },
    send(value) {
      const text = value.trim().slice(0, 500);
      if (!identity || !text || (firstSent && !replied)) return false;
      messages.push({ text, outgoing: true, time: '09:41' });
      firstSent = true;
      return true;
    },
    receive(value) {
      const text = value.trim().slice(0, 500);
      if (!firstSent || !text) return false;
      messages.push({ text, outgoing: false, time: '09:41' });
      replied = true;
      return true;
    },
  };
}

// The notification inbox combines one category with the unread switch.
// Reading a notification only changes the page-local entry supplied by the UI.
export function filterNotifications(entries, category = 'all', unreadOnly = false) {
  return entries.filter(entry => (category === 'all' || entry.category === category) && (!unreadOnly || entry.unread));
}

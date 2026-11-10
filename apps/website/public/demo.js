import { createPlayback, createConversation, filterNotifications } from './demo-playback.js';

// Glyph coordinates follow app_navigation.dart, x_feed.dart and detail_components.dart.
const glyphs = {
  back: '<path d="m11 4.5-7.5 7.5 7.5 7.5M3.8 12H21"/>',
  more: '<g fill="currentColor" stroke="none"><circle cx="12" cy="5.5" r="1.45"/><circle cx="12" cy="12" r="1.45"/><circle cx="12" cy="18.5" r="1.45"/></g>',
  comment: '<path d="M12 3C6.2 3 2.5 6.5 2.5 11c0 3.1 1.7 5.4 4.6 6.8V21l4.1-2.1c5.9.3 10.3-3 10.3-7.9S17.8 3 12 3Z"/>',
  heart: '<path d="M12 21C9.8 19.1 2 13.4 2 7.8 2 2.4 8.5 1.3 12 6c3.5-4.7 10-3.6 10 1.8S14.2 19.1 12 21Z"/>',
  bookmark: '<path d="M6 3h12v18l-6-4-6 4Z"/>',
  share: '<path d="m7.1 10.8 9.7-5.2m-9.7 7.6 9.7 5.2"/><circle cx="5" cy="12" r="2.6"/><circle cx="19" cy="4.5" r="2.6"/><circle cx="19" cy="19.5" r="2.6"/>',
  repost: '<path d="M4.5 13V6.5a2 2 0 0 1 2-2H18m-4-4 4 4-4 4M19.5 11v6.5a2 2 0 0 1-2 2H6m4-4-4 4 4 4"/>',
  views: '<path d="M4 21V12m5 9V7m5 14V3m5 18V10"/>',
  search: '<circle cx="10" cy="10" r="6.5"/><path d="m15 15 6 6"/>',
  chevron: '<path d="m7 9 5 5 5-5"/>',
  plus: '<path d="M12 4v16M4 12h16"/>',
  home: '<path d="m3 12 11-9 11 9v12q0 1-1 1h-7v-8h-6v8H4q-1 0-1-1Z"/>',
  bell: '<path d="M4 21c1.7-3.4 1.5-7.9 2.4-11.7C7.2 5.7 9.9 3.4 14 3.4s6.8 2.3 7.6 5.9c.9 3.8.7 8.3 2.4 11.7ZM10.4 24q3.6 4 7.2 0"/>',
  chat: '<path d="M14 3C7.4 3 3 7.1 3 12.7c0 3.3 1.3 6 3.7 7.8L4.6 25l6.6-2.6C18.9 24.1 25 19.5 25 12.7 25 7.1 20.5 3 14 3Z"/>',
  person: '<circle cx="14" cy="8.5" r="4.5"/><path d="M4 25v-2c0-4.5 4.2-6.3 10-6.3S24 18.5 24 23v2Z"/>',
  image: '<rect x="3" y="3" width="18" height="18" rx="3.4"/><circle cx="15.8" cy="8.2" r="1.4" fill="currentColor" stroke="none"/><path d="m3.7 17.2 4.4-5.1q.6-.7 1.2 0l4.2 4.7 1.9-2.1q.5-.6 1 0l4 4.4"/>',
  expand: '<path d="M14.1 3.9h6v6m-.4-5.6-6 6M9.9 20.1h-6v-6m.4 5.6 6-6"/>',
  send: '<path d="m3 3 19 9-19 9 3-9-3-9Zm3 9h16"/>',
  close: '<path d="m6 6 12 12M6 18 18 6"/>',
  moon: '<path d="M20.5 14a9 9 0 0 1-11-11 9.2 9.2 0 1 0 11 11Z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 1v2m0 18v2M1 12h2m18 0h2M4.2 4.2l1.4 1.4m12.8 12.8 1.4 1.4M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4"/>',
  article: '<path d="M5 3h14v18H5ZM8 7h8M8 11h8M8 15h5"/>',
  lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3"/>',
  attach: '<path d="m9 17 7-9a3 3 0 0 0-4.7-3.7L3.8 14a5 5 0 0 0 7.9 6.2l7.8-10a6.5 6.5 0 0 0-10.3-8.1L2 11.4"/>',
  refresh: '<path d="M20 4v6h-6m5.5-2A8 8 0 1 0 20 16"/>',
  play: '<path d="m8 5 11 7-11 7Z"/>',
  pause: '<path d="M8 5v14M16 5v14"/>',
  // Google's Material Icons outlined waving_hand, used by the Flutter empty state.
  wave: '<path fill="currentColor" stroke="none" d="M7.03 4.95 3.49 8.49c-3.32 3.32-3.32 8.7 0 12.02s8.7 3.32 12.02 0l6.01-6.01c.97-.97.97-2.56 0-3.54-.12-.12-.25-.23-.39-.32l.39-.39c.97-.97.97-2.56 0-3.54-.16-.16-.35-.3-.54-.41.4-.92.23-2.02-.52-2.77-.87-.87-2.22-.96-3.2-.28-.1-.15-.21-.29-.34-.42-.97-.97-2.56-.97-3.54 0l-2.51 2.51c-.09-.14-.2-.27-.32-.39-.97-.97-2.55-.97-3.52 0Zm1.41 1.42c.2-.2.51-.2.71 0s.2.51 0 .71l-3.18 3.18c1.17 1.17 1.17 3.07 0 4.24l1.41 1.41c1.45-1.45 1.82-3.57 1.12-5.36l6.3-6.3c.2-.2.51-.2.71 0s.2.51 0 .71l-4.6 4.6 1.41 1.41 6.01-6.01c.2-.2.51-.2.71 0s.2.51 0 .71l-6.01 6.01 1.41 1.41 4.95-4.95c.2-.2.51-.2.71 0s.2.51 0 .71l-5.66 5.66 1.41 1.41 3.54-3.54c.2-.2.51-.2.71 0s.2.51 0 .71l-6.01 6.01c-2.54 2.54-6.65 2.54-9.19 0s-2.54-6.65 0-9.19l3.54-3.54ZM23 17c0 3.31-2.69 6-6 6v-1.5c2.48 0 4.5-2.02 4.5-4.5H23ZM1 7c0-3.31 2.69-6 6-6v1.5C4.52 2.5 2.5 4.52 2.5 7H1Z"/>',
};
const posts = [
  { id:'study', author:'松间', letter:'松', channel:'搭子', title:'周末想找个安静的自习搭子', body:'打算周六上午去图书馆，各学各的，中午可以一起吃饭。\n有人一起吗？', likes:8, views:126, comments:[{author:'青柠汽水',letter:'柠',body:'我也在准备考试，周六上午可以一起。'},{author:'松间',letter:'松',body:'好呀，九点门口见。',owner:true}] },
  { id:'wind', author:'南岸来信', letter:'南', channel:'日常', title:'傍晚的风，刚刚好', body:'今天没有赶路，绕着江边走了一圈。\n把耳机摘下来，听了很久的风。', likes:12, views:208, comments:[] },
  { id:'book', author:'青柠汽水', letter:'柠', channel:'日常', title:'最近读到的一句话', body:'“生活是自己创造出来的。”\n留一点时间，做真正喜欢的事。', likes:6, views:94, comments:[] },
  { id:'music', author:'一颗橙子', letter:'橙', channel:'日常', title:'分享一首单曲循环', body:'有些歌一响起来，就想起一个很普通、却一直记得的傍晚。', likes:19, views:351, comments:[] },
];
const escape = (text) => String(text).replace(/[&<>"']/g, (char) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
const icon = (name) => `<svg class="demo-icon" aria-hidden="true" viewBox="0 0 ${['home','bell','chat','person'].includes(name) ? 28 : 24} ${['home','bell','chat','person'].includes(name) ? 28 : 24}">${glyphs[name] || ''}</svg>`;
const avatar = (letter, self = false) => `<span class="app-avatar${self ? ' app-avatar-self' : ''}" aria-hidden="true">${escape(letter)}</span>`;

const demo = document.querySelector('#app-demo');
if (demo) initializeDemo(demo);

function initializeDemo(demo) {
  const $ = (selector) => demo.querySelector(selector);
  const $$ = (selector) => [...demo.querySelectorAll(selector)];
  const canvas = $('.demo-canvas');
  const phone = $('.demo-phone');
  const screens = $$('.demo-screen');
  const tabs = $$('[data-step]');
  const motion = matchMedia('(prefers-reduced-motion: reduce)');
  const commentInput = $('#comment-input');
  const messageInput = $('#message-input');
  const replyLayer = $('#demo-reply-layer');
  const animationSet = new Set();
  const scriptedAnimations = new Set();
  let state = initialState();
  let currentScreen = 'welcome';
  let detailId = 'study';
  let contactId = 'study';
  let replyId = 'study';
  let returnFocus = null;
  let manuallyChanged = false;
  let inView = false;
  let fontsReady = false;
  let playback;
  let toastTimer;
  const peerPlaybacks = new Map();
  const scriptedContacts = new Set();
  let notificationCategory = 'all';
  let detailReturn = 'feed';
  const notifications = [
    { id:'reply-study', category:'reply', author:'青柠汽水', letter:'柠', title:'回复了帖子', body:'我也在准备考试，周六上午可以一起。', postId:'study', commentIndex:0, time:'09:36', unread:true },
    { id:'like-study', category:'like', author:'南岸来信', letter:'南', title:'赞了你的回复', body:'周末想找个安静的自习搭子', postId:'study', commentIndex:2, time:'09:34', unread:true },
    { id:'welcome', category:'system', author:'SpeechFUL', letter:'S', title:'欢迎来到 SpeechFUL', body:'从一篇帖子开始，分享日常，也认识同频的人。', time:'昨天', unread:false },
    { id:'community', category:'system', author:'SpeechFUL', letter:'S', title:'一起维护友善的交流空间', body:'尊重不同的想法，也记得保护自己和他人的隐私。', time:'昨天', unread:true },
  ];

  function initialState() {
    return new Map(posts.map((post) => [post.id, { liked:false, bookmarked:false, comments:[...post.comments.map((comment) => ({...comment})), ...(post.id === 'study' ? [{author:'南风',letter:'南',body:'九点见，我带上笔记本。',self:true}] : [])], draft:'', messageDraft:'', conversation:createConversation() }]));
  }
  function getPost(id) { return posts.find((post) => post.id === id) || posts[0]; }
  function hydrateIcons(scope = demo) {
    scope.querySelectorAll('[data-icon]').forEach((element) => { element.innerHTML = icon(element.dataset.icon); });
  }
  function navigation(selected) {
    return `<button type="button" data-go="feed" class="${selected === 'home' ? 'selected' : ''}" ${selected === 'home' ? 'aria-current="page"' : ''} aria-label="首页">${icon('home')}</button><button type="button" data-go="notifications" class="${selected === 'notifications' ? 'selected' : ''}" ${selected === 'notifications' ? 'aria-current="page"' : ''} aria-label="通知">${icon('bell')}</button><span class="app-compose-slot app-display-control" aria-label="发布帖子，仅作界面展示">${icon('plus')}</span><button type="button" data-go="inbox" class="${selected === 'inbox' ? 'selected' : ''}" ${selected === 'inbox' ? 'aria-current="page"' : ''} aria-label="私信">${icon('chat')}</button><span class="app-display-control" aria-label="我的，仅作界面展示">${icon('person')}</span>`;
  }
  function renderNotifications() {
    $$('[data-notification-category]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.notificationCategory === notificationCategory)));
    const entries = filterNotifications(notifications, notificationCategory, $('#demo-notification-unread').checked);
    const list = $('#demo-notification-list');
    list.replaceChildren();
    for (const entry of entries) {
      const row = document.createElement('button'); row.type = 'button'; row.className = 'app-notification-row'; row.dataset.notification = entry.id;
      row.setAttribute('aria-label', `${entry.unread ? '未读，' : ''}${entry.author}，${entry.title}，${entry.body}`);
      row.innerHTML = `${avatar(entry.letter)}<span class="app-notification-text"><strong></strong><span></span><time></time></span>${entry.unread ? '<i class="app-notification-dot" aria-hidden="true"></i>' : ''}`;
      row.querySelector('strong').textContent = entry.category === 'system' ? entry.title : `${entry.author}${entry.title}`;
      row.querySelector('.app-notification-text>span').textContent = entry.body;
      row.querySelector('time').textContent = entry.time;
      list.append(row);
    }
    if (!entries.length) { const empty = document.createElement('p'); empty.className = 'app-notification-empty'; empty.textContent = $('#demo-notification-unread').checked ? '未读消息，都看完了' : '暂时没有新通知'; list.append(empty); }
  }
  function openNotification(id) {
    const entry = notifications.find(item => item.id === id); if (!entry) return;
    entry.unread = false; renderNotifications();
    if (entry.postId) {
      detailReturn = 'notifications'; renderDetail(entry.postId); showScreen('comments');
      const comment = $('#demo-comments').children[entry.commentIndex];
      if (comment) { comment.tabIndex = -1; $('.app-detail-scroll').scrollTop = comment.offsetTop - $('.app-detail-scroll').offsetTop - 12; comment.focus({preventScroll:true}); }
    } else {
      $('#demo-system-title').textContent = entry.title; $('#demo-system-time').textContent = entry.time; $('#demo-system-body').textContent = entry.body;
      showScreen('notification-detail', {focus:true});
    }
  }
  function actions(post, detail = false) {
    const value = state.get(post.id);
    const reply = `<button type="button" data-reply="${post.id}" aria-label="评论">${icon('comment')}<span data-comment-count="${post.id}">${value.comments.length}</span></button>`;
    const heart = `<button type="button" data-like="${post.id}" aria-label="${value.liked ? '取消点赞' : '点赞'}" aria-pressed="${value.liked}">${icon('heart')}<span>${post.likes + Number(value.liked)}</span></button>`;
    const bookmark = `<button type="button" data-bookmark="${post.id}" aria-label="${value.bookmarked ? '取消收藏' : '收藏'}" aria-pressed="${value.bookmarked}">${icon('bookmark')}${detail ? '<span>0</span>' : ''}</button>`;
    const repost = `<span class="app-display-control" aria-label="转发，仅作界面展示">${icon('repost')}</span>`;
    const share = `<span class="app-display-control" aria-label="分享，仅作界面展示">${icon('share')}</span>`;
    const views = `<span aria-label="${post.views}次浏览">${icon('views')}<span>${post.views}</span></span>`;
    return `<div class="app-post-actions${detail ? ' app-detail-actions' : ''}">${(detail ? [reply,heart,bookmark,repost,share] : [reply,repost,heart,views,bookmark,share]).join('')}</div>`;
  }
  function renderFeed(mode = 'recommended') {
    const ordered = mode === 'latest' ? [...posts].reverse() : posts;
    $('#demo-posts').innerHTML = ordered.map((post) => `<article class="app-post" data-post="${post.id}"><button class="app-post-avatar" type="button" data-contact="${post.id}" aria-label="与${escape(post.author)}私信">${avatar(post.letter)}</button><div class="app-post-main"><div class="app-author-line"><button type="button" data-contact="${post.id}" class="app-author">${escape(post.author)}</button><time>· 2026年11月7日</time></div><button class="app-post-open" type="button" data-post-open="${post.id}"><span class="app-post-title">${escape(post.title)}</span><span class="app-post-body">${escape(post.body)}</span></button>${actions(post)}</div><button class="app-icon-button app-post-more" type="button" data-post-open="${post.id}" aria-label="查看帖子">${icon('more')}</button></article>`).join('');
    $$('[data-feed-mode]').forEach((button) => button.setAttribute('aria-pressed', String(button.dataset.feedMode === mode)));
    $('.app-feed-channel').classList.toggle('selected', mode === 'recommended');
    $('.app-feed-scroll').dataset.mode = mode;
  }
  function renderComments() {
    const container = $('#demo-comments');
    container.replaceChildren();
    for (const entry of state.get(detailId).comments) {
      const row = document.createElement('div'); row.className = 'app-comment';
      const badge = document.createElement('span'); badge.className = `app-avatar${entry.self ? ' app-avatar-self' : ''}`; badge.setAttribute('aria-hidden','true'); badge.textContent = entry.letter;
      const content = document.createElement('div');
      const author = document.createElement('strong'); author.textContent = entry.author;
      if (entry.owner) { const owner = document.createElement('em'); owner.textContent = '楼主'; author.append(owner); }
      const text = document.createElement('p'); text.textContent = entry.body;
      const controls = document.createElement('div'); controls.className = 'app-comment-actions'; controls.innerHTML = `<span>${icon('comment')}</span><span>${icon('heart')} 0</span>`;
      content.append(author,text,controls); row.append(badge,content); container.append(row);
    }
    $('.app-comments-end').textContent = state.get(detailId).comments.length ? '没有更多评论' : '暂无评论';
  }
  function renderDetail(id) {
    detailId = id;
    const post = getPost(id);
    $('.app-detail-author .app-avatar').textContent = post.letter;
    $('.app-detail-author .app-author').textContent = post.author;
    $('.app-channel-name').textContent = post.channel;
    $('#demo-detail-title').textContent = post.title;
    $('#demo-detail-body').textContent = post.body;
    $('#demo-views').textContent = post.views;
    $('#demo-detail-actions').innerHTML = actions(post,true);
    renderComments();
  }
  function showScreen(name, { focus = false, animate = true } = {}) {
    const next = screens.find((screen) => screen.dataset.screen === name);
    if (!next) return;
    const changed = currentScreen !== name;
    if (changed && !replyLayer.hidden) closeReply(false);
    currentScreen = name;
    phone.dataset.screen = name;
    screens.forEach((screen) => { screen.hidden = screen !== next; });
    const stepName = name === 'inbox' ? 'messages' : name === 'notification-detail' ? 'notifications' : name;
    tabs.forEach((tab,index) => { const active = tab.dataset.step === stepName; tab.setAttribute('aria-selected',String(active)); tab.tabIndex = active || (index === 0 && !tabs.some(item => item.dataset.step === stepName)) ? 0 : -1; });
    if (name === 'notifications') renderNotifications();
    if (name === 'comments') $('.app-detail-header [data-go]').setAttribute('aria-label', detailReturn === 'notifications' ? '返回通知' : '返回首页');
    $('#demo-tab-messages').setAttribute('aria-controls',name === 'inbox' ? 'demo-inbox' : 'demo-messages-screen');
    $('#demo-tab-notifications').setAttribute('aria-controls',name === 'notification-detail' ? 'demo-notification-detail' : 'demo-notifications');
    if (changed && animate) animateElement(next,[{opacity:0,transform:'translateX(16px)'},{opacity:1,transform:'translateX(0)'}],240);
    if (focus) next.querySelector('.demo-screen-title').focus({preventScroll:true});
  }
  function animateElement(element, keyframes, duration, onFinish) {
    if (motion.matches || !element.animate) { onFinish?.(); return; }
    const animation = element.animate(keyframes,{duration,easing:'cubic-bezier(.2,.75,.25,1)',fill:'none'});
    animationSet.add(animation);
    if (!manuallyChanged) scriptedAnimations.add(animation);
    animation.finished.then(() => { animationSet.delete(animation); scriptedAnimations.delete(animation); onFinish?.(); },() => {animationSet.delete(animation);scriptedAnimations.delete(animation);});
    return animation;
  }
  function takeControl() {
    if (!manuallyChanged && scriptedContacts.size) {
      // Scripted messages demonstrate the UI; a visitor starts a fresh dialogue.
      for (const id of scriptedContacts) {
        const value = state.get(id), identity = value.conversation.identity;
        value.conversation = createConversation();
        if (identity) value.conversation.selectIdentity(identity);
        value.messageDraft = '';
        peerPlaybacks.get(id)?.pause(); peerPlaybacks.delete(id);
      }
      if (scriptedContacts.has(contactId)) renderMessages();
      renderInbox(); scriptedContacts.clear();
    }
    manuallyChanged = true;
    // Settle an in-flight scripted movement before handing scrolling to a person.
    animationSet.forEach((animation) => animation.finish());
    scriptedAnimations.clear();
    playback?.pause();
  }
  function resize() {
    const scale = phone.clientWidth / 402;
    canvas.style.setProperty('--demo-scale',String(scale));
    canvas.style.setProperty('--demo-height',`${phone.clientHeight / scale}px`);
  }
  function updateReady() {
    const visible = inView && fontsReady && !document.hidden;
    playback?.setVisible(visible);
    peerPlaybacks.forEach((peer) => peer.setVisible(visible));
  }
  function toast(text) {
    clearTimeout(toastTimer); const notice = $('#demo-toast'); notice.textContent = text; notice.hidden = false;
    toastTimer = setTimeout(() => { notice.hidden = true; },2200);
    $('#demo-announcement').textContent = text;
  }
  function openReply(id, mode = 'quick', focus = true) {
    replyId = id; returnFocus = document.activeElement;
    $('#demo-reply-person').textContent = getPost(id).author;
    commentInput.value = state.get(id).draft;
    replyLayer.dataset.mode = mode;
    commentInput.placeholder = mode === 'quick' ? '写下你的回复…' : '发布你的回复';
    replyLayer.hidden = false;
    $('.demo-display').inert = true;
    updateReplyButton();
    animateElement($('.app-reply-dialog'),[{opacity:0,transform:mode === 'quick' ? 'scale(.96)' : 'translateY(20px)'},{opacity:1,transform:'none'}],220);
    if (focus) commentInput.focus({preventScroll:true});
  }
  function closeReply(focus = true) {
    state.get(replyId).draft = commentInput.value;
    replyLayer.hidden = true; $('.demo-display').inert = false;
    if (focus && returnFocus?.isConnected && !returnFocus.closest('[hidden]')) returnFocus.focus({preventScroll:true});
  }
  function updateReplyButton() { $('.app-reply-submit').disabled = !commentInput.value.trim(); }
  function submitComment(manual = true) {
    const body = commentInput.value.trim().slice(0,500); if (!body) return;
    state.get(replyId).comments.push({author:'南风',letter:'南',body,self:true});
    state.get(replyId).draft = ''; commentInput.value = '';
    closeReply(false);
    renderFeed($('.app-feed-scroll').dataset.mode);
    if (currentScreen === 'comments') { renderDetail(detailId); $('.app-detail-scroll').scrollTop = $('.app-detail-scroll').scrollHeight; }
    if (manual) {
      const target = returnFocus?.isConnected ? returnFocus : $(`#demo-feed [data-reply="${replyId}"]`);
      target?.focus({preventScroll:true});
    }
    toast('已添加演示回复');
  }
  function renderMessages() {
    const post = getPost(contactId);
    const conversation = state.get(contactId).conversation;
    $('.app-chat-person .app-avatar').textContent = post.letter;
    $('.app-chat-person h3').textContent = post.author;
    $('.app-chat-source').dataset.postOpen = post.id;
    $('.app-chat-source>span:nth-child(2)').textContent = `来自：${post.title}`;
    $('#demo-messages').replaceChildren();
    for (const message of conversation.messages) appendBubble(message);
    $('#demo-chat-empty').hidden = conversation.messages.length > 0;
    $('.app-chat-date').hidden = conversation.messages.length === 0;
    $('#demo-first-contact-note').hidden = !conversation.waiting;
    $('.app-sending-identity [data-icon="lock"]').hidden = !conversation.locked;
    messageInput.value = state.get(contactId).messageDraft;
    messageInput.placeholder = conversation.waiting ? '等待对方回复' : '输入消息';
    messageInput.readOnly = !conversation.identity || conversation.waiting;
    $('.app-attach').hidden = conversation.messages.length === 0 || conversation.waiting;
    updateMessageButton();
    $('.app-chat-scroll').scrollTop = $('.app-chat-scroll').scrollHeight;
  }
  function appendBubble(message) {
    const bubble = document.createElement('p'); bubble.className = `app-chat-bubble${message.outgoing ? ' outgoing' : ''}`;
    bubble.textContent = message.text;
    const time = document.createElement('time'); time.textContent = message.time || '09:41'; bubble.append(time); $('#demo-messages').append(bubble);
    return bubble;
  }
  function sendMessage(manual = true) {
    const text = messageInput.value.trim().slice(0,500); if (!text) return;
    const value = state.get(contactId);
    if (!value.conversation.send(text)) return;
    if (!manual) scriptedContacts.add(contactId);
    value.messageDraft = ''; messageInput.value = '';
    renderMessages(); renderInbox();
    const bubble = $('#demo-messages').lastElementChild;
    $('.app-chat-scroll').scrollTop = $('.app-chat-scroll').scrollHeight;
    animateElement(bubble,[{opacity:0,transform:'translateY(12px)'},{opacity:1,transform:'translateY(0)'}],220);
    $('#demo-announcement').textContent = '消息已添加到本地演示';
    if (manual && value.conversation.waiting) schedulePeerReply(contactId);
  }
  function updateMessageButton() {
    const conversation = state.get(contactId).conversation;
    $('.app-send').disabled = !messageInput.value.trim() || !conversation.identity || conversation.waiting;
  }
  function receiveReply(id) {
    if (!state.get(id).conversation.waiting) return;
    const message = id === 'study' ? '可以的，九点图书馆门口见。' : '你好，很高兴收到你的消息。';
    if (!state.get(id).conversation.receive(message)) return;
    if (contactId === id) {
      renderMessages();
      animateElement($('#demo-messages').lastElementChild,[{opacity:0,transform:'translateY(10px)'},{opacity:1,transform:'none'}],220);
    }
    renderInbox(); $('#demo-announcement').textContent = '虚构对方的回复已显示';
  }
  function schedulePeerReply(id) {
    peerPlaybacks.get(id)?.pause();
    peerPlaybacks.set(id,createPlayback({
      frames:[{duration:2200},{duration:0,reply:true}],
      render(frame){if(frame.reply)receiveReply(id);},status(){},
    }));
    updateReady();
  }
  function beginContact(id, { focus = true, preview = false } = {}) {
    contactId = id; renderMessages();
    if (state.get(id).conversation.identity) { showScreen('messages',{focus}); return; }
    if (preview) showScreen('messages');
    returnFocus = document.activeElement;
    $('#demo-identity-layer').hidden = false;
    $('.demo-display').inert = true;
    animateElement($('.app-identity-sheet'),[{opacity:0,transform:'translateY(32px)'},{opacity:1,transform:'none'}],240);
    if (focus) $('#demo-select-identity').focus({preventScroll:true});
  }
  function closeIdentity(focus = true) {
    $('#demo-identity-layer').hidden = true; $('.demo-display').inert = false;
    if (focus && returnFocus?.isConnected && !returnFocus.closest('[hidden]')) returnFocus.focus({preventScroll:true});
  }
  function selectIdentity(focus = true) {
    state.get(contactId).conversation.selectIdentity('南风');
    closeIdentity(false); renderMessages(); showScreen('messages',{focus});
  }
  function renderInbox() {
    const query = $('#demo-inbox-search').value.trim();
    const entries = posts.filter((post) => state.get(post.id).conversation.messages.length && (!query || `${post.author}\n${post.title}`.includes(query)));
    const list = $('#demo-inbox-list'); list.replaceChildren();
    for (const post of entries) {
      const row = document.createElement('button'); row.type='button'; row.className='app-inbox-row'; row.dataset.contact=post.id;
      row.innerHTML = `${avatar(post.letter)}<span><strong>${escape(post.author)}</strong><small></small></span><time>09:41</time>`;
      row.querySelector('small').textContent = state.get(post.id).conversation.messages.at(-1).text;
      list.append(row);
    }
    if (!entries.length) { const empty=document.createElement('p');empty.className='app-inbox-empty';empty.textContent=query?'没有找到相关会话':'还没有会话';list.append(empty); }
  }
  function reset() {
    animationSet.forEach((animation) => animation.cancel()); animationSet.clear();
    state = initialState(); manuallyChanged = false; detailId = contactId = replyId = 'study';
    peerPlaybacks.forEach((peer) => peer.pause()); peerPlaybacks.clear();
    scriptedAnimations.clear();
    scriptedContacts.clear(); detailReturn = 'feed';
    $('#demo-identity-layer').hidden = true;
    replyLayer.hidden = true; $('.demo-display').inert = false; commentInput.value = messageInput.value = '';
    clearTimeout(toastTimer); $('#demo-toast').hidden = true;
    renderFeed(); renderDetail('study'); renderMessages(); renderInbox();
    $$('.app-feed-scroll,.app-detail-scroll,.app-chat-scroll').forEach((scroll) => { scroll.scrollTop = 0; });
    updateMessageButton(); $('#demo-announcement').textContent = '';
  }
  hydrateIcons();
  $$('[data-navigation]').forEach((element) => { element.innerHTML = navigation(element.dataset.navigation); });
  renderFeed(); renderDetail('study'); renderMessages(); renderInbox(); resize();
  if ('ResizeObserver' in window) new ResizeObserver(resize).observe(phone);
  else window.addEventListener('resize',resize);
  phone.addEventListener('pointerdown',takeControl);
  phone.addEventListener('focusin',takeControl);
  phone.addEventListener('wheel',takeControl,{passive:true});
  phone.addEventListener('touchstart',takeControl,{passive:true});
  demo.addEventListener('click',(event) => {
    const target = event.target.closest('button,[data-close-reply]'); if (!target || !demo.contains(target)) return;
    if (target.dataset.go) { takeControl(); if(target.dataset.go === 'messages') renderMessages(); const destination = currentScreen === 'comments' && target.closest('.app-detail-header') ? detailReturn : target.dataset.go; showScreen(destination,{focus:true}); }
    if (target.dataset.postOpen) { takeControl(); detailReturn = 'feed'; renderDetail(target.dataset.postOpen); $('.app-detail-scroll').scrollTop = 0; showScreen('comments',{focus:true}); }
    if (target.dataset.notificationCategory) { takeControl(); notificationCategory = target.dataset.notificationCategory; renderNotifications(); }
    if (target.dataset.notification) { takeControl(); openNotification(target.dataset.notification); }
    if (target.dataset.reply) { takeControl(); openReply(target.dataset.reply,currentScreen === 'feed' ? 'quick' : 'inline'); }
    if (target.dataset.compose) { takeControl(); openReply(detailId,target.dataset.compose); }
    if (target.hasAttribute('data-close-reply')) closeReply();
    if (target.hasAttribute('data-toggle-expanded')) {
      replyLayer.dataset.mode = replyLayer.dataset.mode === 'expanded' ? 'inline' : 'expanded';
      target.setAttribute('aria-label',replyLayer.dataset.mode === 'expanded' ? '缩小回复输入' : '展开回复输入');
    }
    if (target.dataset.contact || target.hasAttribute('data-detail-contact')) { takeControl(); beginContact(target.dataset.contact || detailId); }
    if (target.dataset.feedMode) { takeControl(); renderFeed(target.dataset.feedMode); $('.app-feed-scroll').scrollTop = 0; }
    if (target.dataset.like || target.dataset.bookmark) {
      takeControl(); const id = target.dataset.like || target.dataset.bookmark; const field = target.dataset.like ? 'liked' : 'bookmarked'; state.get(id)[field] = !state.get(id)[field];
      const selector = target.dataset.like ? `[data-like="${id}"]` : `[data-bookmark="${id}"]`;
      $$(selector).forEach((button) => { button.setAttribute('aria-pressed',String(state.get(id)[field])); button.setAttribute('aria-label',`${state.get(id)[field] ? '取消' : ''}${field === 'liked' ? '点赞' : '收藏'}`); if(field === 'liked') button.querySelector('span').textContent = getPost(id).likes + Number(state.get(id).liked); });
    }
  });
  $('#demo-theme').addEventListener('click',() => { const dark = canvas.dataset.theme !== 'dark'; canvas.dataset.theme = dark ? 'dark' : 'light'; $('#demo-theme').innerHTML = icon(dark ? 'sun' : 'moon'); $('#demo-theme').setAttribute('aria-label',dark ? '切换浅色主题' : '切换深色主题'); });
  $('#demo-language').addEventListener('click',() => { const english = canvas.dataset.language !== 'en'; canvas.dataset.language = english ? 'en' : 'zh'; $$('[data-zh]').forEach((text) => {text.textContent = english ? text.dataset.en : text.dataset.zh;}); $('#demo-language').textContent = english ? '切换为简体中文' : 'Continue in English'; });
  commentInput.addEventListener('input',() => { state.get(replyId).draft = commentInput.value; updateReplyButton(); });
  messageInput.addEventListener('input',()=>{state.get(contactId).messageDraft=messageInput.value;updateMessageButton();});
  $('#demo-inbox-search').addEventListener('input',renderInbox);
  $('#demo-notification-unread').addEventListener('change',() => { takeControl(); renderNotifications(); });
  $('#demo-select-identity').addEventListener('click',()=>selectIdentity());
  $('#demo-identity-layer').addEventListener('click',(event)=>{if(event.target.hasAttribute('data-close-identity'))closeIdentity();});
  $('#demo-identity-layer').addEventListener('keydown',(event)=>{if(event.key==='Escape'){event.preventDefault();closeIdentity();}if(event.key==='Tab'){event.preventDefault();$('#demo-select-identity').focus();}});
  $('#comment-form').addEventListener('submit',(event) => { event.preventDefault(); takeControl(); submitComment(); });
  $('#message-form').addEventListener('submit',(event) => { event.preventDefault(); takeControl(); sendMessage(); });
  replyLayer.addEventListener('keydown',(event) => {
    if(event.key === 'Escape') { event.preventDefault(); closeReply(); }
    if(event.key === 'Tab') { const focusables = [...$('#comment-form').querySelectorAll('button:not(:disabled),textarea')].filter((element)=>element.getClientRects().length); const first=focusables[0],last=focusables.at(-1); if(event.shiftKey && document.activeElement===first){event.preventDefault();last.focus();}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();} }
  });
  tabs.forEach((tab,index) => {
    const select = (button) => { takeControl(); closeIdentity(false); if(!replyLayer.hidden)closeReply(false); if(button.dataset.step==='comments') { detailReturn = 'feed'; renderDetail('study'); } if(button.dataset.step==='messages'){beginContact('study',{preview:true});return;} showScreen(button.dataset.step); };
    tab.addEventListener('click',()=>select(tab)); tab.addEventListener('focus',takeControl);
    tab.addEventListener('keydown',(event)=>{let next; if(event.key==='ArrowRight')next=(index+1)%tabs.length;if(event.key==='ArrowLeft')next=(index+tabs.length-1)%tabs.length;if(event.key==='Home')next=0;if(event.key==='End')next=tabs.length-1;if(next!==undefined){event.preventDefault();select(tabs[next]);tabs[next].focus();}});
  });
  function scrollFeed(top) {
    const scroll = $('.app-feed-scroll');
    const end = Math.min(Math.max(0,top),scroll.scrollHeight-scroll.clientHeight);
    const delta = end-scroll.scrollTop;
    // Animate a compositor transform, then commit the native scroll offset once.
    animateElement($('#demo-posts'),[{transform:'translateY(0)'},{transform:`translateY(${-delta}px)`}],1100,()=>{scroll.scrollTop=end;});
  }
  function typingFrames(screen,input,text,onChange) {
    return [...text].map((_,index)=>({screen,duration:index===text.length-1?600:85,action(){input.value=[...text].slice(0,index+1).join('');onChange?.();}}));
  }
  const frames = [
    {screen:'welcome',duration:2800,action:reset},
    {screen:'feed',duration:2200},
    {screen:'feed',duration:1900,action:()=>scrollFeed(165)},
    {screen:'feed',duration:1500,action:()=>scrollFeed(0)},
    {screen:'feed',duration:1000,action:()=>openReply('study','quick',false)},
    ...typingFrames('feed',commentInput,'我也想去，周六一起吧！',updateReplyButton),
    {screen:'feed',duration:1700,action:()=>submitComment(false)},
    {screen:'comments',duration:1900,action:()=>{renderDetail('study');$('.app-detail-scroll').scrollTop=0;}},
    {screen:'comments',duration:900,action:()=>openReply('study','inline',false)},
    ...typingFrames('comments',commentInput,'我带上笔记本，九点见。',updateReplyButton),
    {screen:'comments',duration:1800,action:()=>submitComment(false)},
    {screen:'comments',duration:1600,action:()=>beginContact('study',{focus:false})},
    {screen:'messages',duration:1700,action:()=>selectIdentity(false)},
    ...typingFrames('messages',messageInput,'你好，周六九点可以吗？',updateMessageButton),
    {screen:'messages',duration:2200,action:()=>sendMessage(false)},
    {screen:'messages',duration:1900,action:()=>receiveReply('study')},
    ...typingFrames('messages',messageInput,'好，周六见！',updateMessageButton),
    {screen:'messages',duration:2400,action:()=>sendMessage(false)},
  ];
  playback = createPlayback({frames,render(frame){frame.action?.();showScreen(frame.screen);},status({playing,done}){
    scriptedAnimations.forEach((animation)=>{if(playing)animation.play();else animation.pause();});
  }});
  document.addEventListener('visibilitychange',updateReady);
  motion.addEventListener('change',()=>{playback.setReduced(motion.matches);if(motion.matches){animationSet.forEach(animation=>animation.finish());}});
  playback.setReduced(motion.matches);
  if('IntersectionObserver' in window)new IntersectionObserver(([entry])=>{inView=entry.isIntersecting&&entry.intersectionRatio>=.4;updateReady();},{threshold:[0,.4]}).observe(phone);
  else {playback.pause();inView=true;}
  // Explicitly load both font families before starting so the first scroll never
  // competes with a font swap. Network access is limited to our own font assets.
  Promise.all([document.fonts.load('17px DemoRoboto'),document.fonts.load('17px DemoNotoSansSC'),document.fonts.ready]).finally(()=>{fontsReady=true;updateReady();});
  $('.demo-controls').hidden=false;
}

import '../localization/app_localizations.dart';

import 'dart:async';

import '../design_system/detail_components.dart';

import '../messaging/dm_runtime.dart';

import 'package:flutter/material.dart';

import '../channels/channel.dart';
import '../design_system/telegram_theme.dart';
import '../identity/active_identity_controller.dart';
import '../identity/identity_avatar.dart';
import '../navigation/app_navigation.dart';
import '../comments/comment_models.dart';
import '../comments/comment_runtime.dart';
import 'feed_api.dart';
import 'feed_controller.dart';
import 'post_controller.dart' hide FeedController;
import 'post_reposts_widgets.dart';
import 'post_sharing.dart';
import 'post_images.dart';
import 'post_screens.dart';
import 'post_search_screen.dart';
import 'post_tag_screen.dart';
import 'post_tag_widgets.dart';
import 'x_feed.dart';
import 'feed_card_menu.dart';
import 'feed_header_glyphs.dart';
import '../reports/report_screens.dart';
import 'x_feed_controls.dart';

/// Production owner: directory, preferences, pagination and protected media.
class FeedScreen extends StatefulWidget {
  const FeedScreen({
    super.key,
    required this.controller,
    required this.channels,
    required this.onManageIdentities,
    required this.onAuthenticationRequired,
    this.onOpenPersonal,
    this.onOpenMessages,
    this.onOpenDirectMessages,
    this.initialChannelId,
    this.loadImages,
    this.showNavigation = true,
  });
  final String? initialChannelId;
  final PostController controller;
  final List<Channel> channels;
  final Future<void> Function() onManageIdentities;
  final VoidCallback onAuthenticationRequired;
  final VoidCallback? onOpenPersonal, onOpenMessages, onOpenDirectMessages;
  final PostImagesLoader? loadImages;
  final bool showNavigation;
  @override
  State<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends State<FeedScreen> {
  /// 与帖子详情里的自动加载保持同一节奏。
  static const _moreThreshold = 320.0;

  late final int _epoch;
  int _publication = 0;
  bool _published = false;
  bool _starting = true;
  bool _latestSelected = false;
  Timer? _notice;
  final _scroll = ScrollController();
  final _seenReplies = <String>{};
  CommentRuntime? _comments;
  final _hiddenHere = <String, String>{};
  int _timelineGeneration = 0;
  String? _feedChannel;
  bool _moreArmed = true;
  bool _moreInFlight = false;
  bool _moreFailed = false;
  bool _viewportFillScheduled = false;
  String? _viewportFillCursor;
  int _viewportFillRuns = 0;
  bool _composeOpen = false;
  bool _searchOpen = false;
  bool _channelPickerOpen = false;
  PostController get c => widget.controller;
  FeedController get feed => c.homeFeed;
  bool get current => mounted && c.available && c.authorityVersion == _epoch;
  List<FeedChannelView> get channels => [
    for (final channel in widget.channels)
      FeedChannelView(channel.id, channel.name, switch (channel.code) {
        'vent' => Icons.chat_bubble_outline,
        'warning' => Icons.warning_amber,
        'recommendation' => Icons.thumb_up_outlined,
        'buddy' => Icons.people_outline,
        'emotion' => Icons.favorite_border,
        'mutual_help' => Icons.volunteer_activism_outlined,
        'clubs' => Icons.groups_outlined,
        'careers' => Icons.work_outline,
        'alumni' => Icons.school_outlined,
        _ => Icons.memory_outlined,
      }),
  ];
  @override
  void initState() {
    super.initState();
    _epoch = c.authorityVersion;
    _publication = c.latestPublicationNotice?.sequence ?? 0;
    _feedChannel = feed.timelineChannelId;
    c.addListener(_changed);
    _scroll.addListener(_onScroll);
    // Starting a new route must not synchronously notify still-mounted sibling
    // routes during Navigator's build. Hide any prior timeline until this read.
    scheduleMicrotask(() async {
      if (!current) return;
      setState(() => _starting = false);
      await feed.loadTimeline(channelId: widget.initialChannelId);
    });
  }

  void _changed() {
    if (!mounted) return;
    if (feed.timelineChannelId != _feedChannel) {
      _feedChannel = feed.timelineChannelId;
      _moreInFlight = false;
      _resetPagination();
    }
    if (feed.timelineGeneration != _timelineGeneration) {
      _timelineGeneration = feed.timelineGeneration;
      _hiddenHere.clear();
      _resetPagination();
    }
    // A normal refresh owns its own error view; it must not leave a stale
    // load-more failure pointing at the previous page.
    if (feed.timelineLoading && !_moreInFlight) _moreFailed = false;
    final notice = c.latestPublicationNotice;
    if (current && notice != null && notice.sequence > _publication) {
      _publication = notice.sequence;
      if (feed.timelineChannelId == null ||
          notice.channelId == feed.timelineChannelId) {
        _published = true;
        _notice?.cancel();
        _notice = Timer(const Duration(seconds: 3), () {
          if (mounted) setState(() => _published = false);
        });
        // Keep the current reading position; explicit refresh gets new posts.
        if (!_scroll.hasClients || _scroll.offset < 20) {
          unawaited(feed.loadTimeline(channelId: feed.timelineChannelId));
        }
      }
    }
    setState(() {});
    if (!feed.timelineLoading &&
        feed.timelineCursor != null &&
        feed.timelineItems.isNotEmpty) {
      _scheduleViewportFill();
    }
  }

  void _onScroll() {
    if (!_scroll.hasClients) return;
    if (_scroll.position.extentAfter >= _moreThreshold) {
      _moreArmed = true;
      return;
    }
    if (!_moreArmed || !_canRequestMore()) return;
    // 同一次接近底部只发一个请求；滚回阈值外再接近才会重新触发。
    _moreArmed = false;
    unawaited(_requestMore());
  }

  bool _canRequestMore({bool retry = false}) =>
      mounted &&
      current &&
      !_starting &&
      !_moreInFlight &&
      !feed.timelineLoading &&
      !feed.preferencesSaving &&
      feed.timelineCursor != null &&
      (retry || feed.timelineError == null);

  Future<void> _requestMore({bool retry = false}) async {
    if (!_canRequestMore(retry: retry)) return;
    final channel = feed.timelineChannelId;
    final generation = feed.timelineGeneration;
    final count = feed.timelineItems.length;
    setState(() {
      _moreInFlight = true;
      _moreFailed = false;
    });
    await feed.loadTimeline(channelId: channel, more: true);
    if (!mounted) return;
    final sameChannel = channel == feed.timelineChannelId;
    final sameGeneration = generation == feed.timelineGeneration;
    setState(() {
      if (sameChannel) _moreInFlight = false;
      if (sameChannel && sameGeneration && feed.timelineError != null) {
        _moreFailed = true;
      }
    });
    // A cursor-invalid recovery reloads the first page; its own generation
    // change already reset the pagination state.
    if (sameChannel &&
        sameGeneration &&
        feed.timelineError == null &&
        feed.timelineItems.length > count) {
      _scheduleViewportFill();
    }
  }

  void _retryMore() {
    if (!_canRequestMore(retry: true)) return;
    _moreArmed = false;
    unawaited(_requestMore(retry: true));
  }

  /// 首屏不足一屏、或追加内容后仍停在触底阈值内时没有新的滚动机会，
  /// 布局完成后补一次检查。
  void _scheduleViewportFill() {
    if (_viewportFillScheduled) return;
    _viewportFillScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _viewportFillScheduled = false;
      if (!mounted || !_canRequestMore()) return;
      if (!_scroll.hasClients) return;
      final position = _scroll.position;
      // 有足够可滚动余量时交给正常的滚动触发，避免首屏就提前请求下一页。
      if (position.maxScrollExtent > 0 &&
          position.extentAfter >= _moreThreshold) {
        return;
      }
      final cursor = feed.timelineCursor;
      // 同一个 cursor 只补载一次；否则服务端返回不足一屏且 cursor 不变的
      // 异常页会让补载在每次通知后无限重试。
      if (cursor == null || cursor == _viewportFillCursor) return;
      if (_viewportFillRuns >= 5) return;
      _viewportFillCursor = cursor;
      _viewportFillRuns++;
      _moreArmed = false;
      unawaited(_requestMore());
    });
  }

  void _resetPagination() {
    _moreArmed = true;
    _moreFailed = false;
    _viewportFillCursor = null;
    _viewportFillRuns = 0;
  }

  @override
  void dispose() {
    _comments?.removeListener(_commentsChanged);
    c.removeListener(_changed);
    _notice?.cancel();
    _scroll.removeListener(_onScroll);
    _scroll.dispose();
    super.dispose();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final runtime = CommentScope.maybeOf(context);
    if (identical(runtime, _comments)) return;
    _comments?.removeListener(_commentsChanged);
    _comments = runtime;
    _seenReplies.clear();
    _seenReplies.addAll(runtime?.core.resolvedTasks.map((t) => t.taskId) ?? []);
    runtime?.addListener(_commentsChanged);
  }

  void _commentsChanged() {
    if (!current || _comments?.available != true) return;
    for (final task in _comments!.core.resolvedTasks) {
      if (task.state == CommentTaskState.published &&
          _seenReplies.add(task.taskId)) {
        unawaited(c.loadInteractions([task.binding.postId], single: true));
      }
    }
  }

  Future<void> _pick(BuildContext context) async {
    if (!current || _channelPickerOpen) return;
    setState(() => _channelPickerOpen = true);
    try {
      await showFeedChannelPicker(
        context,
        channels: channels,
        authority: c,
        isCurrent: () => current,
        selected: feed.timelineChannelId,
        onSelected: (id) {
          if (!current || feed.preferencesSaving) return;
          if (_scroll.hasClients) _scroll.jumpTo(0);
          unawaited(feed.loadTimeline(channelId: id));
        },
      );
    } finally {
      if (mounted) setState(() => _channelPickerOpen = false);
    }
  }

  void _selectFeed(bool latest) {
    if (!current || feed.timelineLoading || feed.preferencesSaving) return;
    setState(() => _latestSelected = latest);
    if (_scroll.hasClients) _scroll.jumpTo(0);
    unawaited(feed.loadTimeline(channelId: feed.timelineChannelId));
  }

  Future<void> _filter(BuildContext context) => showFeedBlockFilter(
    context,
    channels: channels,
    authority: c,
    blocked: () => feed.blockedChannelIds,
    saving: () => feed.preferencesSaving || feed.timelineLoading,
    ready: () => feed.feedPreferencesReady,
    onRetry: () => feed.loadTimeline(channelId: feed.timelineChannelId),
    isCurrent: () => current,
    error: () => feed.preferencesError,
    onChanged: (ids) {
      if (current) unawaited(feed.saveBlockedChannels(ids));
    },
  );
  Future<void> _compose(BuildContext context) async {
    if (!current || _composeOpen) return;
    _composeOpen = true;
    try {
      await _openComposer(context);
    } finally {
      _composeOpen = false;
    }
  }

  Future<void> _openComposer(BuildContext context) async {
    if (!current) return;
    // A new post owns its category; browsing never supplies a default or changes
    // when the author chooses where to publish.
    String? chosen;
    await showFeedChannelPicker(
      context,
      channels: channels,
      authority: c,
      isCurrent: () => current,
      selected: null,
      includeRecommended: false,
      onSelected: (id) => chosen = id,
    );
    if (!mounted || !context.mounted || !current || chosen == null) return;
    final channel = widget.channels.where((p) => p.id == chosen).firstOrNull;
    if (channel == null || !context.mounted) return;
    await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => PostComposerScreen(
          controller: c,
          editor: c.newDraft(channel.id),
          channelName: channel.name,
          onManageIdentities: widget.onManageIdentities,
          onAuthenticationRequired: widget.onAuthenticationRequired,
        ),
      ),
    );
  }

  Future<void> _search(BuildContext context) async {
    if (!current || _searchOpen || !c.supportsPostSearch) return;
    _searchOpen = true;
    final initialChannel = _starting
        ? widget.initialChannelId
        : feed.timelineChannelId;
    try {
      await Navigator.of(context).push<void>(
        MaterialPageRoute(
          builder: (_) => PostSearchScreen(
            controller: c,
            channels: widget.channels,
            initialChannelId: initialChannel,
            onOpenPost: (context, post) =>
                _open(context, post.postId, post.channelId),
            onOpenRankedPost: _open,
            onAuthenticationRequired: widget.onAuthenticationRequired,
          ),
        ),
      );
    } finally {
      _searchOpen = false;
    }
  }

  Future<void> _open(BuildContext context, String id, String channelId) async {
    if (!current) return;
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => PostDetailScreen(
          controller: c,
          postId: id,
          loadImages: widget.loadImages,
          channelName: widget.channels
              .where((p) => p.id == channelId)
              .firstOrNull
              ?.name,
          channelNames: {
            for (final channel in widget.channels) channel.id: channel.name,
          },
          onAuthenticationRequired: widget.onAuthenticationRequired,
        ),
      ),
    );
  }

  Future<void> _openAuthor(BuildContext context, FeedPost post) async {
    if (!current) return;
    await Navigator.of(context).push<void>(
      MaterialPageRoute<void>(
        builder: (_) => AuthorProfileScreen(
          controller: c,
          postId: post.postId,
          channelNames: {
            for (final channel in widget.channels) channel.id: channel.name,
          },
          loadImages: widget.loadImages,
          onAuthenticationRequired: widget.onAuthenticationRequired,
        ),
      ),
    );
  }

  Future<void> _changeHidden(FeedPost post, bool hidden) async {
    if (!current || c.hiddenPostBusy(post.postId)) return;
    final version = await c.changeHiddenPost(
      post.postId,
      hidden,
      version: hidden ? null : _hiddenHere[post.postId],
    );
    if (!mounted || !current) return;
    if (version != null) {
      setState(() {
        if (hidden) {
          _hiddenHere[post.postId] = version;
        } else {
          _hiddenHere.remove(post.postId);
        }
      });
    } else if (c.hiddenPostError case final message?) {
      ScaffoldMessenger.maybeOf(context)
          ?.showSnackBar(SnackBar(content: Text(hhText(context, message))));
    }
  }

  void _reportPost(BuildContext context, FeedPost post) {
    final governance = GovernanceScope.maybeOf(context);
    if (!current || governance?.available != true) return;
    Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => ReportScreen(
          controller: governance!,
          postId: post.postId,
          title: post.title,
        ),
      ),
    );
  }

  /// 列表底部状态只描述分页；首屏错误仍由上方错误视图负责。
  Widget _moreFooter(BuildContext context, List<FeedPost> posts) {
    if (_starting || posts.isEmpty) return const SizedBox.shrink();
    if (_moreInFlight) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
            const SizedBox(width: 8),
            Text(hhText(context, '正在加载')),
          ],
        ),
      );
    }
    if (_moreFailed && feed.timelineError != null) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Flexible(
              child: Text(
                hhText(context, '加载失败，请重试'),
                style: TextStyle(
                  fontSize: 14,
                  color: HhTelegramPalette.of(context).secondary,
                ),
              ),
            ),
            TextButton(
              key: const ValueKey('feed-more-retry'),
              onPressed: _retryMore,
              child: Text(hhText(context, '重试')),
            ),
          ],
        ),
      );
    }
    // `timelineCursor == null` is the controller's end-of-feed contract; it
    // also covers the first page already being the last page.
    if (!feed.timelineLoading &&
        !feed.preferencesSaving &&
        feed.timelineCursor == null &&
        feed.timelineError == null) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Center(
          child: Text(
            hhText(context, '已经到底'),
            style: TextStyle(
              fontSize: 14,
              color: HhTelegramPalette.of(context).secondary,
            ),
          ),
        ),
      );
    }
    return const SizedBox.shrink();
  }

  @override
  Widget build(BuildContext context) => Theme(
    data: HhTelegramTheme.create(
      Theme.of(context).brightness,
      fontFamily: 'Roboto',
    ),
    child: Builder(
      builder: (context) {
        if (!current) {
          return Scaffold(
            body: Center(
              child: TextButton(
                onPressed: widget.onAuthenticationRequired,
                child: Text(hhText(context, '登录或核对会话')),
              ),
            ),
          );
        }
        final posts = _starting ? const <FeedPost>[] : feed.timelineItems;
        final selectedChannel = widget.channels
            .where((channel) => channel.id == feed.timelineChannelId)
            .firstOrNull;
        return HhXPostFeed(
          showNavigation: widget.showNavigation,
          accountAvatar: c.activeIdentity == null
              ? null
              : _FeedAccountAvatar(controller: c.activeIdentity!),
          loading:
              posts.isNotEmpty &&
              (feed.timelineLoading || feed.preferencesSaving),
          refreshEnabled:
              !_starting && !feed.timelineLoading && !feed.preferencesSaving,
          onRefresh: () async {
            if (!current || feed.timelineLoading || feed.preferencesSaving) {
              return;
            }
            await feed.loadTimeline(channelId: feed.timelineChannelId);
          },
          onLatest: _starting || feed.timelineLoading || feed.preferencesSaving
              ? null
              : () => _selectFeed(true),
          onRecommended:
              _starting || feed.timelineLoading || feed.preferencesSaving
              ? null
              : () => _selectFeed(false),
          latestSelected: _latestSelected,
          channelPickerOpen: _channelPickerOpen,
          channelTitle: selectedChannel?.name ?? '推荐',
          feedTitle: selectedChannel?.name ?? '推荐',
          composeTooltip: '选择分类写帖子',
          blockedCount: feed.blockedChannelIds.length,
          posts: const [],
          mediaBuilder: (_, _) => const SizedBox.shrink(),
          onPost: (_) {},
          onAuthor: (_) {},
          onAction: (_, _) {},
          onMedia: (_, _) {},
          onMenu: (_) {},
          onBack: () => _pick(context),
          onSearch: c.supportsPostSearch ? () => _search(context) : null,
          onFilter: () => _filter(context),
          onCompose: () => _compose(context),
          onDestination: (destination) {
            switch (destination) {
              case FeedDestination.home:
                // This is already the home route. Keep its channel, loaded
                // pages and reading position when the home icon is tapped.
                return;
              case FeedDestination.personal:
                widget.onOpenPersonal?.call();
              case FeedDestination.messages:
                widget.onOpenMessages?.call();
              case FeedDestination.directMessages:
                widget.onOpenDirectMessages?.call();
            }
          },
          content: Stack(
            children: [
              ListView(
                controller: _scroll,
                key: const ValueKey('production-feed'),
                physics: const AlwaysScrollableScrollPhysics(
                  parent: ClampingScrollPhysics(),
                ),
                padding: EdgeInsets.only(
                  bottom: 96 + MediaQuery.paddingOf(context).bottom,
                ),
                children: [
                  if ((_starting || feed.timelineLoading) && posts.isEmpty)
                    const FeedLoadingPlaceholder(),
                  if (!_starting &&
                      feed.timelineError != null &&
                      (posts.isEmpty || !_moreFailed))
                    Padding(
                      padding: const EdgeInsets.all(24),
                      child: Column(
                        children: [
                          Text(hhText(context, feed.timelineError!)),
                          TextButton(
                            onPressed: () => feed.loadTimeline(
                              channelId: feed.timelineChannelId,
                            ),
                            child: Text(hhText(context, '重新加载')),
                          ),
                        ],
                      ),
                    ),
                  if (!_starting &&
                      !feed.timelineLoading &&
                      !feed.preferencesSaving &&
                      feed.timelineError == null &&
                      posts.isEmpty)
                    Padding(
                      padding: const EdgeInsets.symmetric(
                        vertical: 80,
                        horizontal: 24,
                      ),
                      child: Column(
                        children: [
                          Text(hhText(context, '暂无可显示的帖子')),
                          if (feed.blockedChannelIds.isNotEmpty)
                            TextButton(
                              onPressed: () => _filter(context),
                              child: Text(hhText(context, '调整屏蔽设置')),
                            ),
                        ],
                      ),
                    ),
                  for (final post in posts) ...[
                    if (_hiddenHere.containsKey(post.postId))
                      FeedHiddenUndoRow(
                        key: ValueKey('hidden-placeholder:${post.postId}'),
                        label: hhText(context, '撤销'),
                        onUndo: c.hiddenPostBusy(post.postId)
                            ? null
                            : () => unawaited(_changeHidden(post, false)),
                      )
                    else
                      HhFeedPostCard(
                        key: ValueKey('post:${post.postId}'),
                        postId: post.postId,
                        flat: true,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            PostShareAction(
                              controller: c,
                              postId: post.postId,
                              builder: (share, sharing) => PostRepostButton(
                                controller: c,
                                postId: post.postId,
                                builder: (repost, reposted, unresolved) => PostFlatCard(
                                  key: ValueKey('flat-post:${post.postId}'),
                                  onOpen: () => _open(
                                    context,
                                    post.postId,
                                    post.channelId,
                                  ),
                                  onMenu: () => _open(
                                    context,
                                    post.postId,
                                    post.channelId,
                                  ),
                                  menuWidget: FeedCardMenu(
                                    postId: post.postId,
                                    tooltip: hhText(context, '帖子更多操作'),
                                    shareLabel: hhText(
                                      context,
                                      sharing ? '正在准备分享…' : '分享',
                                    ),
                                    shareEnabled: share != null,
                                    repostLabel: hhText(
                                      context,
                                      unresolved ? '重试核对' : '转帖',
                                    ),
                                    showRepost: c.supportsReposts,
                                    repostEnabled: repost != null,
                                    reposted: reposted,
                                    notInterestedLabel: hhText(context, '不感兴趣'),
                                    reportLabel: hhText(context, '举报'),
                                    notInterestedEnabled:
                                        c.supportsHiddenPosts &&
                                        !c.hiddenPostBusy(post.postId),
                                    reportEnabled:
                                        GovernanceScope.maybeOf(context)
                                            ?.available ==
                                        true,
                                    onSelected: (action) {
                                      if (!current) return;
                                      switch (action) {
                                        case FeedCardMenuAction.share:
                                          share?.call();
                                        case FeedCardMenuAction.repost:
                                          repost?.call();
                                        case FeedCardMenuAction.notInterested:
                                          unawaited(_changeHidden(post, true));
                                        case FeedCardMenuAction.report:
                                          _reportPost(context, post);
                                      }
                                    },
                                  ),
                                  author: localizedPostAuthor(
                                    context,
                                    post.author,
                                  ),
                                  elapsed: postDateLabel(
                                    context,
                                    post.publishedAt,
                                  ),
                                  avatar: DetailAvatar(
                                    name: localizedPostAuthor(
                                      context,
                                      post.author,
                                    ),
                                    avatar: post.author.avatar,
                                    radius: 15,
                                  ),
                                  onAuthor: () => unawaited(
                                    DmScope.contact(context, post.postId),
                                  ),
                                  onAvatar: () =>
                                      unawaited(_openAuthor(context, post)),
                                  tags: post.tags.isEmpty
                                      ? null
                                      : PostTagChips(
                                          tags: post.tags,
                                          onTap: (tag) => unawaited(
                                            openPostTag(
                                              context,
                                              controller: c,
                                              tag: tag,
                                              channels: widget.channels,
                                              onAuthenticationRequired: widget
                                                  .onAuthenticationRequired,
                                              loadImages: widget.loadImages,
                                            ),
                                          ),
                                        ),
                                  title: post.title,
                                  body: post.body,
                                  bleedMedia: true,
                                  media:
                                      post.imageCount > 0 &&
                                          widget.loadImages != null
                                      ? PostImagesSection(
                                          key:
                                              c.postMediaRevision(
                                                    post.postId,
                                                  ) ==
                                                  0
                                              ? ValueKey(
                                                  'feed-images:${post.postId}',
                                                )
                                              : ValueKey((
                                                  post.postId,
                                                  c.postMediaRevision(
                                                    post.postId,
                                                  ),
                                                )),
                                          postId: post.postId,
                                          feedPreview: true,
                                          feedPreviewInset: 20,
                                          initialSizes:
                                              post.imageSizes
                                                  ?.map(
                                                    (size) => Size(
                                                      size.width.toDouble(),
                                                      size.height.toDouble(),
                                                    ),
                                                  )
                                                  .toList() ??
                                              List.filled(
                                                post.imageCount,
                                                const Size(1, 1),
                                              ),
                                          expectedCount: post.imageCount,
                                          loadImages: widget.loadImages!,
                                          authority: c,
                                          isCurrent: () =>
                                              current &&
                                              !c.isPostDeleted(post.postId),
                                        )
                                      : null,
                                  actions: [
                                    PostFlatAction(
                                      key: ValueKey('like:${post.postId}'),
                                      icon: c.interaction(post.postId).liked
                                          ? Icons.favorite_rounded
                                          : Icons.favorite_border_rounded,
                                      on: c.interaction(post.postId).liked,
                                      selectedColor: HhTelegramPalette.of(
                                        context,
                                      ).error,
                                      label:
                                          '${c.interaction(post.postId).likes ?? '—'}',
                                      semanticsLabel: hhText(context, '点赞'),
                                      onTap:
                                          c
                                                  .interaction(post.postId)
                                                  .canChange &&
                                              !sharing
                                          ? () {
                                              if (!current) return;
                                              unawaited(
                                                c.changeInteraction(
                                                  post.postId,
                                                  PostInteractionAction.like,
                                                ),
                                              );
                                            }
                                          : null,
                                    ),
                                    PostFlatAction(
                                      key: ValueKey('bookmark:${post.postId}'),
                                      icon: c.interaction(post.postId).favorited
                                          ? Icons.bookmark_rounded
                                          : Icons.bookmark_border_rounded,
                                      on: c.interaction(post.postId).favorited,
                                      label:
                                          '${c.interaction(post.postId).value?.favoriteCount ?? '—'}',
                                      semanticsLabel: hhText(context, '收藏'),
                                      onTap:
                                          c.interaction(post.postId).canChange
                                          ? () {
                                              if (!current) return;
                                              unawaited(
                                                c.changeInteraction(
                                                  post.postId,
                                                  PostInteractionAction
                                                      .favorite,
                                                ),
                                              );
                                            }
                                          : null,
                                    ),
                                    PostFlatAction(
                                      key: ValueKey('comment:${post.postId}'),
                                      glyph: FeedActionIcon(
                                        action: FeedAction.comment,
                                        color: HhTelegramPalette.of(context)
                                            .secondary,
                                        size: 17,
                                      ),
                                      label:
                                          c
                                              .interaction(post.postId)
                                              .value
                                              ?.commentCount
                                              .toString() ??
                                          '—',
                                      semanticsLabel: hhText(context, '评论'),
                                      onTap: () => _open(
                                        context,
                                        post.postId,
                                        post.channelId,
                                      ),
                                    ),
                                    if (share != null)
                                      PostFlatAction(
                                        key: ValueKey('share:${post.postId}'),
                                        icon: Icons.ios_share_rounded,
                                        label: '',
                                        semanticsLabel: hhText(
                                          context,
                                          sharing ? '正在准备分享…' : '分享',
                                        ),
                                        onTap: sharing ? null : share,
                                      ),
                                  ],
                                ),
                              ),
                            ),
                            for (final notice in [
                              (c.interaction(post.postId).writeError, true),
                              (c.interaction(post.postId).readError, false),
                            ])
                              if (notice.$1 case final error?)
                                Padding(
                                  padding: const EdgeInsets.only(
                                    left: 64,
                                    right: 16,
                                  ),
                                  child: Row(
                                    children: [
                                      Expanded(
                                        child: Text(hhText(context, error)),
                                      ),
                                      TextButton(
                                        key: ValueKey(
                                          'interaction-${notice.$2 ? '' : 'read-'}retry:${post.postId}',
                                        ),
                                        onPressed:
                                            c.interaction(post.postId).sending
                                            ? null
                                            : () {
                                                if (!current) return;
                                                if (notice.$2) {
                                                  unawaited(
                                                    c.retryInteraction(
                                                      post.postId,
                                                    ),
                                                  );
                                                } else {
                                                  unawaited(
                                                    c.loadInteractions([
                                                      post.postId,
                                                    ], single: true),
                                                  );
                                                }
                                              },
                                        child: Text(
                                          notice.$2
                                              ? hhText(context, '重试')
                                              : hhText(context, '重新读取'),
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                          ],
                        ),
                      ),
                  ],
                  _moreFooter(context, posts),
                ],
              ),
              if (_published)
                Positioned(
                  top: 12,
                  left: 80,
                  right: 80,
                  child: Material(
                    color: HhTelegramPalette.of(context).action,
                    borderRadius: const BorderRadius.all(Radius.circular(8)),
                    child: Padding(
                      padding: const EdgeInsets.all(12),
                      child: Text(
                        hhText(context, '发布成功！'),
                        textAlign: TextAlign.center,
                        style: const TextStyle(color: Colors.white),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        );
      },
    ),
  );
}

class _FeedAccountAvatar extends StatelessWidget {
  const _FeedAccountAvatar({required this.controller});

  final ActiveIdentityController controller;

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: controller,
    builder: (context, _) {
      final identity = controller.selectedIdentity;
      return IdentityAvatar(
        avatar: identity?.avatar ?? 'default-v1',
        radius: 16,
        fallback: const Center(child: FeedHeaderAvatarGlyph()),
      );
    },
  );
}

/// Presentation only: it never mounts controllers or reads protected data.
/// Match the live home layout while session/directory authority is restored.
class FeedStartupScreen extends StatelessWidget {
  const FeedStartupScreen({super.key});

  @override
  Widget build(BuildContext context) => Semantics(
    label: hhText(context, '正在准备首页'),
    child: ExcludeSemantics(
      child: ExcludeFocus(
        child: IgnorePointer(
          child: Scaffold(
            key: const ValueKey('startup-feed'),
            bottomNavigationBar: HhAppNavigation(
              composeSlot: true,
              onCompose: () {},
              selected: AppDestination.home,
              onSelected: (_) {},
            ),
            body: Theme(
              data: HhTelegramTheme.create(
                Theme.of(context).brightness,
                fontFamily: 'Roboto',
              ),
              child: HhXPostFeed(
                showNavigation: false,
                channelTitle: '',
                feedTitle: '推荐',
                composeTooltip: '选择分类写帖子',
                posts: const [],
                mediaBuilder: (_, _) => const SizedBox.shrink(),
                onPost: (_) {},
                onAuthor: (_) {},
                onAction: (_, _) {},
                onMedia: (_, _) {},
                onMenu: (_) {},
                onBack: () {},
                onSearch: () {},
                onFilter: () {},
                onCompose: () {},
                onDestination: (_) {},
                content: ListView(
                  padding: const EdgeInsets.only(bottom: 96),
                  children: const [FeedLoadingPlaceholder()],
                ),
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

class FeedLoadingPlaceholder extends StatelessWidget {
  const FeedLoadingPlaceholder({super.key});

  @override
  Widget build(BuildContext context) => Semantics(
    label: hhText(context, '正在加载帖子'),
    child: ExcludeSemantics(
      child: Column(
        children: [
          for (var i = 0; i < 2; i++)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  CircleAvatar(
                    backgroundColor: Theme.of(context)
                        .colorScheme
                        .surfaceContainerHighest,
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        _bar(context, 120, 16),
                        const SizedBox(height: 12),
                        _bar(context, double.infinity, 14),
                        const SizedBox(height: 8),
                        _bar(context, 160, 14),
                        const SizedBox(height: 12),
                        _bar(context, double.infinity, 150),
                      ],
                    ),
                  ),
                ],
              ),
            ),
        ],
      ),
    ),
  );

  Widget _bar(BuildContext context, double width, double height) => Container(
    width: width,
    height: height,
    decoration: BoxDecoration(
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      borderRadius: BorderRadius.circular(8),
    ),
  );
}

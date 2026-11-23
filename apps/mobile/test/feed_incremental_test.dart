import 'dart:async';

import 'package:flutter/material.dart';

import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/posts/feed_api.dart';
import 'package:hnuhole_mobile/src/posts/feed_controller.dart';
import 'package:hnuhole_mobile/src/posts/post_api.dart';
import 'package:hnuhole_mobile/src/posts/post_models.dart';

import 'feed_ui_test.dart' as feed;
import 'identity_test_support.dart';

class _Api extends feed.FeedTestApi implements IncrementalFeedApi {
  final requests = <({String? channel, String? older, String? refresh})>[];
  final pages = <FeedWindow>[];
  Completer<PostResponse<FeedWindow>>? pending;
  PostFailure? failure;
  @override
  Future<PostResponse<FeedWindow>> timelineWindow({
    required String sessionToken,
    String? channelId,
    String? cursor,
    String? refreshCursor,
  }) async {
    requests.add((channel: channelId, older: cursor, refresh: refreshCursor));
    if (pending != null) return pending!.future;
    final failure = this.failure;
    this.failure = null;
    if (failure != null) throw failure;
    return feed.reply(pages.removeAt(0));
  }
}

class _Session implements FeedReadSession {
  @override
  bool get isCurrent => true;
  @override
  Future<T> request<T>(Future<PostResponse<T>> Function(String) send) async =>
      (await send('session')).value;
  @override
  void reportFailure(Object error) {}
}

String _id(int n) => '00000000-0000-4000-8000-${n.toString().padLeft(12, '0')}';
FeedWindow _page(
  Iterable<int> ids, {
  String? older,
  String? refresh,
  int? after,
}) => FeedWindow(
  items: ids.map((n) => feed.post(id: _id(n))).toList(),
  nextCursor: older,
  refreshCursor: refresh,
  insertAfter: after == null ? null : _id(after),
);

/// Minimal-height posts: used to exercise a short appended page while the
/// viewport is still scrollable.
FeedWindow _shortPage(Iterable<int> ids, {String? older}) => FeedWindow(
  items: [
    for (final n in ids)
      FeedPost(
        postId: _id(n),
        channelId: feed.channelId,
        title: '',
        body: '',
        author: feed.author,
        publishedAt: feed.at,
        viewCount: 1,
      ),
  ],
  nextCursor: older,
  refreshCursor: null,
  insertAfter: null,
);
FeedController _controller(_Api api) => FeedController(
  api: api,
  available: () => true,
  openSession: () async => _Session(),
  invalidateInteractionReads: () {},
  loadInteractions: (_) async => true,
  isPostDeleted: (_) => false,
);

const _nextAuthor = PostAuthor(
  state: PostAuthorState.active,
  avatar: 'avatar-v1:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
  nickname: '更新后的身份',
);

FeedPost _refreshed(FeedPost old, {int viewCount = 1}) => FeedPost(
  postId: old.postId,
  channelId: old.channelId,
  title: old.title,
  author: _nextAuthor,
  publishedAt: old.publishedAt,
  body: old.body,
  viewCount: viewCount,
  imageCount: old.imageCount,
  tags: old.tags,
);

void main() {
  testWidgets('pull refresh fills gaps and load more retains the old cursor', (
    tester,
  ) async {
    final api = _Api()
      ..pages.addAll([
        _page(
          [16, 15, 14, 13, 12, 11, 10, 9, 8, 7],
          older: 'old-1',
          refresh: 'head-1',
        ),
        _page([28, 27, 26, 25, 24], refresh: 'gap-1'),
        _page([23, 22, 21, 20, 19], refresh: 'gap-2', after: 24),
        _page([18, 17], refresh: 'head-2', after: 19),
        _page([6, 5, 4, 3, 2]),
      ]);
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.length, 10);
    final list = find.byKey(const ValueKey('production-feed'));
    for (final expected in [15, 20, 22]) {
      await tester.drag(list, const Offset(0, 350));
      await tester.pumpAndSettle();
      expect(fixture.controller.homeFeed.timelineItems.length, expected);
      expect(fixture.controller.homeFeed.timelineCursor, 'old-1');
    }
    await tester.drag(list, const Offset(0, -20000));
    await tester.pumpAndSettle();
    expect(
      api.requests.where((r) => r.older == 'old-1').length,
      1,
      reason: 'reaching the bottom auto-loads exactly one older page',
    );
    expect(fixture.controller.homeFeed.timelineItems.map((p) => p.postId), [
      for (var n = 28; n >= 2; n--) _id(n),
    ]);
    expect(api.requests.last.older, 'old-1');
    expect(api.requests.last.refresh, isNull);
    expect(fixture.controller.homeFeed.timelineCursor, isNull);
    expect(tester.takeException(), isNull);
  });

  testWidgets('bottom auto-load reports pending and end states once', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final api = _Api()
      ..pages.add(_page([10, 9, 8, 7, 6, 5, 4, 3, 2, 1], older: 'older'));
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.length, 10);

    api.pending = Completer<PostResponse<FeedWindow>>();
    final list = find.byKey(const ValueKey('production-feed'));
    await tester.drag(list, const Offset(0, -20000));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('正在加载'), findsOneWidget);
    expect(find.byKey(const ValueKey('feed-more-retry')), findsNothing);
    expect(api.requests.where((r) => r.older == 'older').length, 1);

    api.pending!.complete(
      feed.reply(
        FeedWindow(
          items: const [],
          nextCursor: null,
          refreshCursor: null,
          insertAfter: null,
        ),
      ),
    );
    api.pending = null;
    await tester.pumpAndSettle();
    expect(find.text('正在加载'), findsNothing);
    expect(find.text('已经到底'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('a single-page timeline reports the end on first load', (
    tester,
  ) async {
    final api = _Api()..pages.add(_page([10, 9, 8, 7, 6, 5, 4, 3, 2, 1]));
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    // The first page is already the last page; no older request is needed.
    expect(api.requests.length, 1);
    expect(fixture.controller.homeFeed.timelineCursor, isNull);
    await tester.drag(
      find.byKey(const ValueKey('production-feed')),
      const Offset(0, -20000),
    );
    await tester.pumpAndSettle();
    expect(api.requests.length, 1);
    expect(find.text('已经到底'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('a short older page keeps loading without another drag', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(390, 500));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final api = _Api()
      ..pages.addAll([
        _shortPage([10, 9, 8, 7, 6, 5, 4, 3, 2, 1], older: 'older-1'),
        _shortPage([11], older: 'older-2'),
        _shortPage([12]),
      ]);
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    expect(api.requests.length, 1);

    final list = find.byKey(const ValueKey('production-feed'));
    await tester.drag(list, const Offset(0, -20000));
    await tester.pumpAndSettle();
    // 第二页只有一条短内容；追加后仍在触底阈值内，应自动继续下一页。
    expect(api.requests.length, 3);
    expect(api.requests.map((r) => r.older).whereType<String>(), [
      'older-1',
      'older-2',
    ]);
    expect(fixture.controller.homeFeed.timelineCursor, isNull);
    expect(find.text('已经到底'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('load-more failure keeps posts and retries from the footer', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final api = _Api()
      ..pages.addAll([
        _page([10, 9, 8, 7, 6, 5, 4, 3, 2, 1], older: 'older'),
        _page([20, 19, 18, 17, 16]),
      ]);
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    final generation = fixture.controller.homeFeed.timelineGeneration;

    api.failure = const PostFailure(kind: PostFailureKind.transport);
    final list = find.byKey(const ValueKey('production-feed'));
    await tester.drag(list, const Offset(0, -20000));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.length, 10);
    expect(find.text('加载失败，请重试'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('feed-more-retry')));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.length, 15);
    expect(fixture.controller.homeFeed.timelineCursor, isNull);
    expect(fixture.controller.homeFeed.timelineError, isNull);
    expect(fixture.controller.homeFeed.timelineGeneration, generation);
    expect(find.text('加载失败，请重试'), findsNothing);
    // The new rows push the footer below the viewport; scroll to it again.
    await tester.drag(list, const Offset(0, -20000));
    await tester.pumpAndSettle();
    expect(find.text('已经到底'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('a short first page fills without a scroll gesture', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final api = _Api()
      ..pages.addAll([
        _page([3, 2], older: 'older'),
        _page([1]),
      ]);
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.map((p) => p.postId), [
      _id(3),
      _id(2),
      _id(1),
    ]);
    expect(api.requests.length, 2);
    expect(api.requests.last.older, 'older');
    expect(find.text('已经到底'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('a cursor that never advances cannot loop the viewport fill', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final api = _Api()
      ..pages.addAll([
        _page([2, 1], older: 'same'),
        _page(const [], older: 'same'),
      ]);
    final fixture = feed.FeedFixture(await IdentityTestFixture.create(), api);
    addTearDown(fixture.dispose);
    await tester.pumpWidget(feed.screen(fixture));
    await tester.pumpAndSettle();
    expect(api.requests.length, 2);
    expect(tester.takeException(), isNull);
  });

  test('initial ten, incremental newest five and gap insertion preserve old cursor', () async {
    final api = _Api()
      ..pages.addAll([
        _page(
          [16, 15, 14, 13, 12, 11, 10, 9, 8, 7],
          older: 'old-1',
          refresh: 'head-1',
        ),
        _page([28, 27, 26, 25, 24], refresh: 'gap-1'),
        _page([23, 22, 21, 20, 19], refresh: 'gap-2', after: 24),
        _page([18, 17], refresh: 'head-2', after: 19),
        _page([], refresh: 'head-3'),
        _page([6, 5, 4, 3, 2], older: 'old-2'),
      ]);
    final c = _controller(api);
    addTearDown(c.dispose);
    for (var n = 0; n < 5; n++) {
      await c.loadTimeline();
    }
    expect(c.timelineItems.map((p) => p.postId), [
      for (var n = 28; n >= 7; n--) _id(n),
    ]);
    expect(c.timelineCursor, 'old-1');
    expect(api.requests.map((r) => r.refresh), [
      null,
      'head-1',
      'gap-1',
      'gap-2',
      'head-2',
    ]);
    await c.loadTimeline(more: true);
    expect(api.requests.last.older, 'old-1');
    expect(api.requests.last.refresh, isNull);
    expect(c.timelineItems.length, 27);
    expect(c.timelineCursor, 'old-2');
  });
  test(
    'displayed projection refresh patches rows in place across pages',
    () async {
      final api = _Api()..pages.add(_page([3, 2, 1], refresh: 'head'));
      final c = _controller(api);
      addTearDown(c.dispose);
      await c.loadTimeline();
      const nextAuthor = PostAuthor(
        state: PostAuthorState.active,
        avatar: 'avatar-v1:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
        nickname: '更新后的身份',
      );
      FeedPost refreshed(int n) {
        final old = feed.post(id: _id(n));
        return FeedPost(
          postId: old.postId,
          channelId: old.channelId,
          title: old.title,
          author: nextAuthor,
          publishedAt: old.publishedAt,
          body: old.body,
          viewCount: 1,
          imageCount: old.imageCount,
          tags: old.tags,
        );
      }

      api.pageFor = (channel, cursor) => PostsPage(
        items: cursor == null
            ? [refreshed(4), refreshed(3)]
            : [refreshed(2), refreshed(1)],
        nextCursor: cursor == null ? 'next-page' : null,
      );
      await c.refreshDisplayedProjections();
      expect(c.timelineItems.map((post) => post.postId), [
        _id(3),
        _id(2),
        _id(1),
      ]);
      expect(
        c.timelineItems.every((post) => post.author.nickname == '更新后的身份'),
        isTrue,
      );
      expect(c.timelineItems.any((post) => post.postId == _id(4)), isFalse);
      expect(c.postViewCount(_id(3)), 1260);
      expect(api.calls.where((call) => call.startsWith('feed:')).length, 2);
    },
  );
  test(
    'projection refresh keeps a larger locally observed view count',
    () async {
      final api = _Api()..pages.add(_page([1], refresh: 'head'));
      final c = _controller(api);
      addTearDown(c.dispose);
      await c.loadTimeline();
      expect(c.postViewCount(_id(1)), 1260);
      api.views = 5000;
      await c.registerVisiblePost(_id(1));
      expect(c.postViewCount(_id(1)), 5000);

      api.pageFor = (channel, cursor) => PostsPage(
        items: [_refreshed(feed.post(id: _id(1)), viewCount: 1)],
        nextCursor: null,
      );
      await c.refreshDisplayedProjections();

      expect(c.postViewCount(_id(1)), 5000);
      expect(c.timelineItems.single.author.nickname, _nextAuthor.nickname);
      expect(c.timelineItems.single.viewCount, 5000);
    },
  );
  test(
    'projection refresh follows page shifts until the oldest displayed row',
    () async {
      final displayed = [
        for (var n = 20; n >= 1; n--)
          feed.post(
            id: _id(n),
            publishedAt: feed.at.add(Duration(minutes: n)),
          ),
      ];
      final api = _Api()
        ..pages.add(
          FeedWindow(
            items: displayed,
            nextCursor: 'displayed-older',
            refreshCursor: null,
            insertAfter: null,
          ),
        );
      final c = _controller(api);
      addTearDown(c.dispose);
      await c.loadTimeline();
      expect(c.timelineItems, hasLength(20));

      final server = [
        for (var i = 1; i <= 41; i++)
          feed.post(
            id: _id(1000 + i),
            publishedAt: feed.at.add(Duration(minutes: 100 - i)),
          ),
        for (final post in displayed) _refreshed(post),
        for (var i = 0; i < 19; i++)
          feed.post(
            id: _id(2000 + i),
            publishedAt: feed.at.subtract(Duration(minutes: i + 1)),
          ),
      ];
      api.pageFor = (channel, cursor) {
        final start = switch (cursor) {
          null => 0,
          'page-2' => 20,
          'page-3' => 40,
          'page-4' => 60,
          _ => throw StateError('unexpected cursor $cursor'),
        };
        final end = start + 20 > server.length ? server.length : start + 20;
        return PostsPage(
          items: server.sublist(start, end),
          nextCursor: 'page-${start ~/ 20 + 2}',
        );
      };

      await c.refreshDisplayedProjections();

      expect(c.timelineItems.map((post) => post.postId), [
        for (var n = 20; n >= 1; n--) _id(n),
      ]);
      expect(
        c.timelineItems.every(
          (post) => post.author.nickname == _nextAuthor.nickname,
        ),
        isTrue,
      );
      // Page 4 ends past the oldest displayed row, so page 5 must not load.
      expect(api.calls.where((call) => call.startsWith('feed:')).length, 4);
    },
  );
  test('account reset fences an in-flight incremental result', () async {
    final api = _Api()..pages.add(_page([1], refresh: 'head'));
    final c = _controller(api);
    addTearDown(c.dispose);
    await c.loadTimeline();
    final pending = Completer<PostResponse<FeedWindow>>();
    api.pending = pending;
    final request = c.loadTimeline();
    await Future<void>.delayed(Duration.zero);
    c.reset();
    pending.complete(feed.reply(_page([2], refresh: 'next')));
    await request;
    expect(c.timelineItems, isEmpty);
    expect(c.timelineCursor, isNull);
  });
  test('channel and filter changes start a new initial window', () async {
    final api = _Api()
      ..pages.addAll([
        _page([1], refresh: 'head'),
        _page([2], refresh: 'channel-head'),
      ]);
    final c = _controller(api);
    addTearDown(c.dispose);
    await c.loadTimeline();
    await c.loadTimeline(channelId: feed.channelId);
    expect(api.requests.last.refresh, isNull);
    expect(api.requests.last.older, isNull);
    expect(c.timelineItems.single.postId, _id(2));
  });
}

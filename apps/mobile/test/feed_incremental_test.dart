import 'dart:async';

import 'package:flutter/material.dart';

import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/posts/feed_api.dart';
import 'package:hnuhole_mobile/src/posts/feed_controller.dart';
import 'package:hnuhole_mobile/src/posts/post_api.dart';

import 'feed_ui_test.dart' as feed;
import 'identity_test_support.dart';

class _Api extends feed.FeedTestApi implements IncrementalFeedApi {
  final requests = <({String? channel, String? older, String? refresh})>[];
  final pages = <FeedWindow>[];
  Completer<PostResponse<FeedWindow>>? pending;
  @override
  Future<PostResponse<FeedWindow>> timelineWindow({
    required String sessionToken,
    String? channelId,
    String? cursor,
    String? refreshCursor,
  }) async {
    requests.add((channel: channelId, older: cursor, refresh: refreshCursor));
    if (pending != null) return pending!.future;
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
FeedController _controller(_Api api) => FeedController(
  api: api,
  available: () => true,
  openSession: () async => _Session(),
  invalidateInteractionReads: () {},
  loadInteractions: (_) async => true,
  isPostDeleted: (_) => false,
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
    await tester.ensureVisible(find.text('加载更多'));
    await tester.tap(find.text('加载更多'));
    await tester.pumpAndSettle();
    expect(fixture.controller.homeFeed.timelineItems.map((p) => p.postId), [
      for (var n = 28; n >= 2; n--) _id(n),
    ]);
    expect(api.requests.last.older, 'old-1');
    expect(api.requests.last.refresh, isNull);
    expect(fixture.controller.homeFeed.timelineCursor, isNull);
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

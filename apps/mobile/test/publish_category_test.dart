import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/posts/x_feed.dart';

import 'feed_ui_test.dart' as feed;
import 'identity_test_support.dart';

// Exercise the shipped screens and navigation with in-memory save/publish ports.
// These checks do not claim real storage, HTTP or server publication coverage.
class _ComposeController extends feed.FeedTestController {
  _ComposeController(super.auth, super.api);
  final drafts = <PostDraftEditor>[];
  PostDraftEditor? saved, published;

  @override
  PostDraftEditor newDraft(String channelId) {
    final editor = super.newDraft(channelId);
    drafts.add(editor);
    return editor;
  }

  @override
  Future<bool> saveDraft(PostDraftEditor editor) async {
    saved = editor;
    editor.saved = true;
    return true;
  }

  @override
  Future<List<ManagedIdentity>> prepareComposer(PostDraftEditor editor) async {
    editor.identityId ??= identityTestId;
    return [
      ManagedIdentity(
        id: identityTestId,
        nickname: '测试身份',
        avatar: 'default-v1',
        isOriginal: true,
        createdAt: feed.at,
      ),
    ];
  }

  @override
  Future<bool> publish(PostDraftEditor editor) async {
    published = editor;
    announce(editor.channelId);
    return true;
  }
}

Widget _app(_ComposeController controller, {String? browseCategory}) =>
    MaterialApp(
      home: FeedScreen(
        controller: controller,
        channels: feed.channels,
        initialChannelId: browseCategory,
        onManageIdentities: () async {},
        onAuthenticationRequired: () {},
      ),
    );

void _expectUnselectedPicker(WidgetTester tester) {
  expect(find.text('选择帖子分类'), findsOneWidget);
  expect(find.byType(PostComposerScreen), findsNothing);
  expect(
    find.byKey(const ValueKey('choose-channel:recommended')),
    findsNothing,
  );
  final semantics = tester.widgetList<Semantics>(
    find.descendant(
      of: find.byKey(const ValueKey('channel-options')),
      matching: find.byType(Semantics),
    ),
  );
  expect(semantics.where((s) => s.properties.selected == true), isEmpty);
}

void main() {
  for (final browseCategory in [null, ...feed.channels.map((c) => c.id)]) {
    testWidgets(
      'new post requires an independent category from ${browseCategory ?? 'recommended'}',
      (tester) async {
        final auth = await IdentityTestFixture.create();
        final api = feed.FeedTestApi();
        final controller = _ComposeController(auth, api);
        addTearDown(controller.dispose);
        addTearDown(auth.dispose);
        await tester.pumpWidget(
          _app(controller, browseCategory: browseCategory),
        );
        await tester.pumpAndSettle();
        final calls = List.of(api.calls);

        await tester.tap(find.byKey(const ValueKey('feed-compose')));
        await tester.pumpAndSettle();
        _expectUnselectedPicker(tester);
        expect(controller.drafts, isEmpty);
        await tester.binding.handlePopRoute();
        await tester.pumpAndSettle();
        expect(controller.drafts, isEmpty);
        expect(controller.timelineChannelId, browseCategory);

        await tester.tap(find.byKey(const ValueKey('feed-compose')));
        await tester.pumpAndSettle();
        await tester.tap(
          find.byKey(const ValueKey('choose-channel:${feed.otherChannelId}')),
        );
        await tester.pumpAndSettle();
        final composer = tester.widget<PostComposerScreen>(
          find.byType(PostComposerScreen),
        );
        expect(composer.editor.channelId, feed.otherChannelId);
        expect(composer.channelName, '避雷');
        expect(controller.timelineChannelId, browseCategory);
        expect(api.calls, calls);

        await tester.tap(find.byTooltip('返回'));
        await tester.pumpAndSettle();
        expect(controller.saved?.channelId, feed.otherChannelId);
        expect(controller.timelineChannelId, browseCategory);
        expect(
          tester.widget<HhXPostFeed>(find.byType(HhXPostFeed)).feedTitle,
          browseCategory == null
              ? '推荐'
              : feed.channels.firstWhere((c) => c.id == browseCategory).name,
        );
        await tester.tap(find.byKey(const ValueKey('feed-compose')));
        await tester.pumpAndSettle();
        _expectUnselectedPicker(tester);
        expect(controller.drafts, hasLength(1));
        await tester.pumpWidget(const SizedBox());
        await tester.pump();
      },
    );
  }

  for (final browseCategory in [null, feed.channelId]) {
    testWidgets(
      'cross-category publication preserves pages and position from ${browseCategory ?? 'recommended'}',
      (tester) async {
        final auth = await IdentityTestFixture.create();
        final api = feed.FeedTestApi();
        api.pageFor = (category, cursor) => PostsPage(
          items: [
            for (
              var i = cursor == null ? 1 : 10;
              i < (cursor == null ? 10 : 13);
              i++
            )
              feed.post(
                id: '33333333-3333-4333-8333-${i.toString().padLeft(12, '0')}',
                title: '浏览帖子$i',
                channel: category ?? feed.channelId,
              ),
          ],
          nextCursor: cursor == null ? 'page-two' : 'page-three',
        );
        final controller = _ComposeController(auth, api);
        addTearDown(controller.dispose);
        addTearDown(auth.dispose);
        await tester.pumpWidget(
          _app(controller, browseCategory: browseCategory),
        );
        await tester.pumpAndSettle();
        await tester.tap(find.byKey(const ValueKey('feed-latest')));
        await tester.pumpAndSettle();
        await controller.loadTimeline(channelId: browseCategory, more: true);
        await tester.pumpAndSettle();
        final list = find.byKey(const ValueKey('production-feed'));
        final scroll = tester.widget<ListView>(list).controller!;
        scroll.jumpTo(280);
        await tester.pumpAndSettle();
        final offset = scroll.offset;
        final items = List.of(controller.timelineItems);
        final generation = controller.timelineGeneration;
        final calls = List.of(api.calls);
        expect(items, hasLength(12));
        expect(controller.timelineCursor, 'page-three');

        await tester.tap(find.byKey(const ValueKey('feed-compose')));
        await tester.pumpAndSettle();
        await tester.tap(
          find.byKey(const ValueKey('choose-channel:${feed.otherChannelId}')),
        );
        await tester.pumpAndSettle();
        await tester.enterText(find.byType(TextField).at(0), '独立分类测试');
        await tester.enterText(find.byType(TextField).at(1), '发布分类来自本次明确选择。');
        await tester.tap(find.text('下一步'));
        await tester.pumpAndSettle();
        expect(find.text('发布确认'), findsOneWidget);
        expect(find.text('避雷'), findsOneWidget);
        await tester.tap(find.text('确认发布'));
        await tester.pumpAndSettle();

        expect(find.byType(PostComposerScreen), findsNothing);
        expect(controller.published?.channelId, feed.otherChannelId);
        expect(controller.published?.title, '独立分类测试');
        expect(controller.timelineChannelId, browseCategory);
        expect(controller.timelineItems, items);
        expect(controller.timelineCursor, 'page-three');
        expect(controller.timelineGeneration, generation);
        expect(scroll.offset, offset);
        expect(
          tester.widget<HhXPostFeed>(find.byType(HhXPostFeed)).latestSelected,
          isTrue,
        );
        expect(api.calls, calls);
        await tester.pumpWidget(const SizedBox());
        await tester.pump();
      },
    );

    testWidgets(
      'publication at the top refreshes only its matching browse scope ${browseCategory ?? 'recommended'}',
      (tester) async {
        final auth = await IdentityTestFixture.create();
        final api = feed.FeedTestApi();
        final controller = _ComposeController(auth, api);
        addTearDown(controller.dispose);
        addTearDown(auth.dispose);
        await tester.pumpWidget(
          _app(controller, browseCategory: browseCategory),
        );
        await tester.pumpAndSettle();
        api.calls.clear();
        controller.announce(feed.channelId);
        await tester.pumpAndSettle();
        expect(controller.timelineChannelId, browseCategory);
        expect(api.calls.where((call) => call.startsWith('feed:')), [
          'feed:${browseCategory ?? 'recommended'}:first:20',
        ]);
        expect(find.text('发布成功！'), findsOneWidget);
        expect(find.text('今天的校园'), findsOneWidget);
        await tester.pumpWidget(const SizedBox());
        await tester.pump();
      },
    );
  }

  testWidgets('logout while choosing a posting category closes authorization', (
    tester,
  ) async {
    final auth = await IdentityTestFixture.create();
    final controller = _ComposeController(auth, feed.FeedTestApi());
    addTearDown(controller.dispose);
    addTearDown(auth.dispose);
    await tester.pumpWidget(_app(controller, browseCategory: feed.channelId));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('feed-compose')));
    await tester.pumpAndSettle();
    _expectUnselectedPicker(tester);
    await auth.sessions.logout();
    await tester.pumpAndSettle();
    expect(find.text('请重新登录'), findsOneWidget);
    expect(find.text('选择帖子分类'), findsNothing);
    expect(
      find.byKey(const ValueKey('choose-channel:${feed.channelId}')),
      findsNothing,
    );
    expect(controller.drafts, isEmpty);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byType(PostComposerScreen), findsNothing);
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
  });
}

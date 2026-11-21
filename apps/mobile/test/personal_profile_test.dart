import 'dart:async';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/design_system/motion.dart';
import 'package:hnuhole_mobile/src/identity/identity_avatar.dart';

import 'identity_test_support.dart';

import 'package:hnuhole_mobile/src/posts/x_feed.dart';

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:hnuhole_mobile/src/posts/feed_header_glyphs.dart';
import 'package:flutter_test/flutter_test.dart';

import 'login_home_test_support.dart';
import 'feed_ui_test.dart' as feed;
import 'personal_reposts_integration_test.dart';

import 'package:hnuhole_mobile/hnuhole_mobile.dart';

Finder profileTab(int index) => find.byKey(ValueKey('profile-tab-$index'));

void expectProfileSelection(WidgetTester tester, int selected) {
  const labels = ['帖子', '回复', '转帖', '喜欢', '收藏'];
  final palette = HhTelegramPalette.of(tester.element(profileTab(selected)));
  for (var i = 0; i < labels.length; i++) {
    // 原型 .ptab：图标与 11px 文字常显；选中态用 accent + w600。
    expect(find.text(labels[i]), findsOneWidget);
    final node = tester.getSemantics(profileTab(i));
    final data = node.getSemanticsData();
    expect(data.label, contains(labels[i]));
    expect(data.flagsCollection.isSelected.toBoolOrNull(), i == selected);
    expect(data.hasAction(SemanticsAction.tap), isTrue);
  }
  expect(
    tester.widget<Text>(find.text(labels[selected])).style!.color,
    palette.accent,
  );
  expect(
    tester
        .widget<ColoredBox>(find.byKey(const ValueKey('profile-tab-indicator')))
        .color,
    palette.accent,
  );
}

void main() {
  testWidgets(
    'rapid personal categories keep real selection when navigation flags change',
    (tester) async {
      final media = ValueNotifier(const MediaQueryData(size: Size(800, 600)));
      final semantics = tester.ensureSemantics();
      final f = HomeTestFixture();
      await tester.pumpWidget(
        ValueListenableBuilder(
          valueListenable: media,
          child: await f.create(),
          builder: (_, value, child) => MediaQuery(data: value, child: child!),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      for (final tab in [3, 4, 1, 2, 0]) {
        await tester.tap(profileTab(tab));
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 16));
        for (var index = 0; index < 5; index++) {
          expect(
            tester.widget<HhAnimatedSelection>(profileTab(index)).selected,
            index == tab,
          );
        }
      }
      media.value = media.value.copyWith(accessibleNavigation: true);
      await tester.pumpAndSettle();
      expectProfileSelection(tester, 0);
      for (final tab in [4, 3, 0]) {
        await tester.tap(profileTab(tab));
        await tester.pumpAndSettle();
        expectProfileSelection(tester, tab);
      }
      await tester.tap(find.byTooltip('切换身份'));
      await tester.pump();
      final rotation = tester.widget<AnimatedRotation>(
        find.byType(AnimatedRotation),
      );
      expect(rotation.duration, HhMotion.quick);
      final route = ModalRoute.of(tester.element(find.text('切换身份')))!;
      expect(route.transitionDuration, greaterThan(Duration.zero));
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(find.text('切换身份'), findsNothing);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      media.dispose();
      semantics.dispose();
    },
  );

  testWidgets(
    'home avatar follows identity selection and same-identity updates',
    (tester) async {
      final f = HomeTestFixture();
      final root = await f.create();
      const second = '22222222-2222-4222-8222-222222222222';
      const firstAvatar = 'avatar-v1:33333333-3333-4333-8333-333333333333';
      const nextAvatar = 'avatar-v1:44444444-4444-4444-8444-444444444444';
      f.identitiesApi.items = [
        identityTestItem(name: '甲', avatar: firstAvatar),
        identityTestItem(id: second, name: '乙', original: false),
      ];
      await tester.pumpWidget(root);
      await tester.pumpAndSettle();
      final account = find.byKey(const ValueKey('feed-account'));
      IdentityAvatar avatar() => tester.widget<IdentityAvatar>(
        find.descendant(of: account, matching: find.byType(IdentityAvatar)),
      );
      expect(avatar().avatar, firstAvatar);
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('切换身份'));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('profile-identity-$second')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('destination:home')));
      await tester.pumpAndSettle();
      expect(avatar().avatar, 'default-v1');
      expect(
        find.descendant(
          of: account,
          matching: find.byType(FeedHeaderAvatarGlyph),
        ),
        findsOneWidget,
      );
      f.identitiesApi.items[1] = identityTestItem(
        id: second,
        name: '乙',
        avatar: nextAvatar,
        original: false,
      );
      await root.identities.load();
      await tester.pumpAndSettle();
      expect(avatar().avatar, nextAvatar);
      root.identities.cancelView();
      await tester.pumpAndSettle();
      expect(root.identities.directory, isNull);
      expect(avatar().avatar, nextAvatar);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );

  for (final brightness in Brightness.values) {
    testWidgets(
      'personal uses same global palette as home and inbox: $brightness',
      (tester) async {
        tester.platformDispatcher.platformBrightnessTestValue = brightness;
        addTearDown(tester.platformDispatcher.clearPlatformBrightnessTestValue);
        final f = HomeTestFixture();
        final root = await f.create();
        final post = feed.post(title: '主题一致的帖子');
        f.postApi.identityItems[identityTestId] = [
          OwnPostCard(post: post, serverSortAt: post.publishedAt),
        ];
        await tester.pumpWidget(root);
        await tester.pumpAndSettle();
        final palette = HhTelegramPalette.of(
          tester.element(find.byType(HhXPostFeed)),
        );
        await tester.tap(find.byKey(const ValueKey('destination:personal')));
        await tester.pumpAndSettle();
        final profile = find.byType(PersonalPostsScreen);
        final scaffold = find
            .descendant(of: profile, matching: find.byType(Scaffold))
            .first;
        expect(
          tester.widget<Scaffold>(scaffold).backgroundColor,
          palette.surface,
        );
        expect(
          tester
              .widget<Text>(find.byKey(const ValueKey('profile-hero-name')))
              .style!
              .color,
          palette.ink,
        );
        expect(find.byTooltip('搜索我的帖子'), findsNothing);
        expect(find.byTooltip('打开设置'), findsOneWidget);
        expect(find.byType(TextField), findsNothing);
        final semantics = tester.ensureSemantics();
        expectProfileSelection(tester, 0);
        semantics.dispose();
        final body = find.text('主题一致的帖子');
        expect(Theme.of(tester.element(body)).brightness, brightness);
        expect(
          HhTelegramPalette.of(tester.element(body)).surface,
          palette.surface,
        );
        expect(find.byKey(const ValueKey('personal-mood')), findsNothing);
        await tester.tap(find.byTooltip('切换身份'));
        await tester.pumpAndSettle();
        expect(
          HhTelegramPalette.of(tester.element(find.text('切换身份'))).surface,
          palette.surface,
        );
        await tester.tap(
          find.byKey(const ValueKey('profile-identity-$identityTestId')),
        );
        await tester.pumpAndSettle();
        f.postApi.identityItems[identityTestId] = [];
        await f.posts.loadPersonal();
        await tester.pumpAndSettle();
        final emptyAction = tester.widget<IconButton>(
          find.byWidgetPredicate(
            (widget) => widget is IconButton && widget.tooltip == '选择分类发帖',
          ),
        );
        expect((emptyAction.icon as Icon).color, palette.action);
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.pump();
      },
    );
  }

  testWidgets(
    'only the selected tab label animates in blue on narrow screens; collection ownership stays intact',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      tester.platformDispatcher.textScaleFactorTestValue = 1.6;
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
      final semantics = tester.ensureSemantics();
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      const second = '22222222-2222-4222-8222-222222222222';
      f.identitiesApi.items.add(
        identityTestItem(id: second, name: '身份二', original: false),
      );
      final firstPost = feed.post(title: '第一身份的帖子');
      final secondPost = feed.post(
        id: '33333333-3333-4333-8333-333333333334',
        title: '第二身份的帖子',
      );
      f.postApi.identityItems[identityTestId] = [
        OwnPostCard(post: firstPost, serverSortAt: firstPost.publishedAt),
      ];
      f.postApi.identityItems[second] = [
        OwnPostCard(post: secondPost, serverSortAt: secondPost.publishedAt),
      ];
      f.postApi.items = [firstPost, secondPost];
      f.postApi.likedItems = [firstPost];
      f.postApi.favoriteItems = [secondPost];
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      tester.view.physicalSize = const Size(320, 740);
      await tester.pumpAndSettle();
      expectProfileSelection(tester, 0);
      expect(find.byKey(const ValueKey('profile-settings')), findsOneWidget);
      expect(find.byIcon(Icons.swap_horiz), findsNothing);
      final chip = find.byKey(const ValueKey('personal-profile'));
      expect(
        find.descendant(
          of: chip,
          matching: find.byIcon(Icons.chevron_right_rounded),
        ),
        findsOneWidget,
      );
      expect(find.text('第一身份的帖子'), findsOneWidget);
      final small = tester
          .widget<Icon>(
            find
                .descendant(of: profileTab(3), matching: find.byType(Icon))
                .first,
          )
          .size!;
      await tester.tap(profileTab(3));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 60));
      final middle = tester
          .widget<Icon>(
            find
                .descendant(of: profileTab(3), matching: find.byType(Icon))
                .first,
          )
          .size!;
      expect(middle, greaterThan(small));
      expect(middle, lessThan(21));
      await tester.pumpAndSettle();
      // 原型 .ptab：标签文字常显 11px；选中态用颜色/字重表达（v8 起）。
      expect(tester.widget<Text>(find.text('喜欢')).style!.fontSize, 11);
      expectProfileSelection(tester, 3);
      await tester.tap(find.byTooltip('切换身份'));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('profile-identity-$second')));
      await tester.pumpAndSettle();
      expect(find.text('身份二'), findsNWidgets(2));
      expect(find.text('第一身份的帖子'), findsOneWidget); // account-shared likes
      await tester.tap(profileTab(4));
      await tester.pumpAndSettle();
      expectProfileSelection(tester, 4);
      expect(find.text('第二身份的帖子'), findsOneWidget);
      await tester.tap(profileTab(0));
      await tester.pumpAndSettle();
      expectProfileSelection(tester, 0);
      expect(find.text('第一身份的帖子'), findsNothing);
      expect(find.text('第二身份的帖子'), findsOneWidget);
      // Complete the older identity response after switching back: it cannot overwrite.
      final pending = Completer<PostResponse<PostsPage<OwnPostCard>>>();
      f.postApi.identityWaits[identityTestId] = pending;
      final old = f.posts.selectPersonalIdentity(identityTestId);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text('第二身份的帖子'), findsNothing);
      await f.posts.selectPersonalIdentity(second);
      pending.complete(
        feed.reply(
          PostsPage(
            items: f.postApi.identityItems[identityTestId]!,
            nextCursor: null,
          ),
        ),
      );
      await old;
      await tester.pumpAndSettle();
      expect(find.text('第二身份的帖子'), findsOneWidget);
      expect(find.text('第一身份的帖子'), findsNothing);
      f.postApi.identityWaits.clear();
      await f.posts.selectPersonalIdentity(identityTestId);
      await tester.pumpAndSettle();
      await tester.tap(profileTab(4));
      await tester.pumpAndSettle();
      expect(find.text('第二身份的帖子'), findsOneWidget); // account-shared favorites
      for (final tab in [1, 2]) {
        await tester.tap(profileTab(tab));
        await tester.pumpAndSettle();
        expectProfileSelection(tester, tab);
      }
      expect(tester.takeException(), isNull);
      semantics.dispose();
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );
  testWidgets(
    'profile scrolls away while tabs pin and posts remain a single column',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      final posts = List.generate(
        16,
        (i) => feed.post(
          id: '33333333-3333-4333-8333-${i.toString().padLeft(12, '0')}',
        ),
      );
      f.postApi.items = posts;
      f.postApi.ownItems = posts
          .map((p) => OwnPostCard(post: p, serverSortAt: p.publishedAt))
          .toList();
      // 目标页签也要有足够内容，才能验证切页后的落点。
      f.postApi.likedItems = posts;
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      // 五页签卡片已按原型改为扁平样式（_ProfileFlatCard），不再使用 HhXPostItem。
      expect(find.byType(HhXPostItem), findsNothing);
      expect(find.byKey(const ValueKey('flat-post-card')), findsWidgets);
      await tester.drag(
        find.byType(CustomScrollView).last,
        const Offset(0, -550),
      );
      await tester.pumpAndSettle();
      final tabsY = tester.getTopLeft(find.byTooltip('帖子排序')).dy;
      expect(tabsY, lessThan(12));
      expect(find.byKey(const ValueKey('personal-profile')), findsNothing);
      await tester.drag(
        find.byType(CustomScrollView).last,
        const Offset(0, -250),
      );
      await tester.pumpAndSettle();
      expect(tester.getTopLeft(find.byTooltip('帖子排序')).dy, tabsY);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );
  testWidgets('a committed author block clears the loaded favorites page', (
    tester,
  ) async {
    final f = HomeTestFixture();
    const favoritePostId = '66666666-6666-4666-8666-666666666666';
    const blockedAuthor = PostAuthor(
      state: PostAuthorState.active,
      avatar: 'default-v1',
      nickname: '被拉黑作者',
    );
    f.postApi.favoriteItems = [
      PostCard(
        postId: favoritePostId,
        channelId: feed.channelId,
        title: '收藏的帖子',
        author: blockedAuthor,
        publishedAt: feed.at,
      ),
    ];
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('destination:personal')));
    await tester.pumpAndSettle();
    await tester.tap(profileTab(4));
    await tester.pumpAndSettle();
    expect(find.text('收藏的帖子'), findsOneWidget);

    // The author profile commits a block while the favorites page stays
    // mounted underneath. Its cached card belongs to the old visibility
    // window and must be discarded before the tab reload completes.
    f.postApi.items = [feed.post(id: favoritePostId)];
    f.postApi.favoriteItems = [];
    expect(await f.posts.blockAuthor(favoritePostId), isTrue);
    await tester.pumpAndSettle();
    expect(find.text('收藏的帖子'), findsNothing);
    expect(find.text('还没有收藏的帖子'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
  });

  testWidgets(
    'profile card menu keeps the post detail actions, not a silent detail jump',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = _OwnPostApi();
      final f = HomeTestFixture(api: api);
      final root = await f.create();
      final post = feed.post();
      api.items = [post];
      api.ownItems = [OwnPostCard(post: post, serverSortAt: post.publishedAt)];
      await tester.pumpWidget(root);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('帖子操作').first);
      await tester.pumpAndSettle();
      expect(find.byType(PostDetailScreen), findsNothing);
      expect(find.text('编辑帖子'), findsOneWidget);
      expect(find.text('删除帖子'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );

  testWidgets('like never reports uncertainty only because it is in flight', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(393, 852);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final api = _OwnPostApi();
    final f = HomeTestFixture(api: api);
    final root = await f.create();
    final post = feed.post();
    api.items = [post];
    api.ownItems = [OwnPostCard(post: post, serverSortAt: post.publishedAt)];
    await tester.pumpWidget(root);
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('destination:personal')));
    await tester.pumpAndSettle();
    final gate = Completer<void>();
    api.interactionApi.beforeCas = gate;
    await tester.tap(find.byKey(ValueKey('profile-like-${post.postId}')));
    await tester.pump();
    expect(find.text('结果尚未确认，请重试核对。'), findsNothing);
    gate.complete();
    api.interactionApi.beforeCas = null;
    await tester.pumpAndSettle();
    expect(find.text('结果尚未确认，请重试核对。'), findsNothing);
    expect(api.interactionApi.changes, isNotEmpty);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
  });
  testWidgets(
    'profile comment opens the post and a like turns the heart red',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = _OwnPostApi();
      final f = HomeTestFixture(api: api);
      final root = await f.create();
      final post = feed.post();
      api.items = [post];
      api.ownItems = [OwnPostCard(post: post, serverSortAt: post.publishedAt)];
      await tester.pumpWidget(root);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();

      // 评论入口与首页同款图标，点击只打开帖子。
      expect(find.byType(FeedActionIcon), findsWidgets);
      await tester.tap(
        find.byKey(ValueKey('profile-comment-${post.postId}')),
      );
      await tester.pumpAndSettle();
      expect(find.byType(PostDetailScreen), findsOneWidget);
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();

      final like = find.byKey(ValueKey('profile-like-${post.postId}'));
      await tester.tap(like);
      await tester.pumpAndSettle();
      final heart = tester.widget<Icon>(
        find.descendant(of: like, matching: find.byType(Icon)),
      );
      expect(heart.icon, Icons.favorite_rounded);
      expect(heart.color, HhTelegramPalette.of(tester.element(like)).error);
      expect(api.interactionApi.actual.likedByMe, isTrue);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );

  testWidgets(
    'switching to an empty tab shows only that tab and settles without a late jump',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      final posts = List.generate(
        16,
        (i) => feed.post(
          id: '33333333-3333-4333-8333-${i.toString().padLeft(12, '0')}',
          title: '帖子$i',
        ),
      );
      f.postApi.items = posts;
      f.postApi.ownItems = posts
          .map((p) => OwnPostCard(post: p, serverSortAt: p.publishedAt))
          .toList();
      // 喜欢页签没有任何内容。
      f.postApi.likedItems = [];
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      final scroll = tester
          .widget<CustomScrollView>(find.byType(CustomScrollView).last)
          .controller!;
      await tester.drag(
        find.byType(CustomScrollView).last,
        const Offset(0, -550),
      );
      await tester.pumpAndSettle();
      expect(scroll.offset, greaterThan(300));

      await tester.tap(profileTab(3));
      await tester.pumpAndSettle();
      // 空页签只显示自己的空状态，不残留上一页签的卡片。
      expect(find.text('还没有喜欢的帖子'), findsOneWidget);
      expect(find.text('帖子0'), findsNothing);
      // 空白区覆盖页签栏以下的可用高度：停在“头像收起”的位置，不回弹到顶部。
      final resting = scroll.offset;
      expect(resting, greaterThan(100));
      expect(resting, lessThan(530));
      await tester.drag(
        find.byType(CustomScrollView).last,
        const Offset(0, -200),
      );
      await tester.pumpAndSettle();
      expect(scroll.offset, closeTo(resting, 0.5));
      expect(find.text('还没有喜欢的帖子'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );

  testWidgets(
    'switching tabs keeps the same deep scroll position (no jump to the top)',
    (tester) async {
      tester.view.physicalSize = const Size(393, 852);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      final posts = List.generate(
        16,
        (i) => feed.post(
          id: '33333333-3333-4333-8333-${i.toString().padLeft(12, '0')}',
          title: '帖子$i',
        ),
      );
      f.postApi.items = posts;
      f.postApi.ownItems = posts
          .map((p) => OwnPostCard(post: p, serverSortAt: p.publishedAt))
          .toList();
      f.postApi.likedItems = posts;
      await tester.tap(find.byKey(const ValueKey('destination:personal')));
      await tester.pumpAndSettle();
      final scroll = tester
          .widget<CustomScrollView>(find.byType(CustomScrollView).last)
          .controller!;
      await tester.drag(
        find.byType(CustomScrollView).last,
        const Offset(0, -550),
      );
      await tester.pumpAndSettle();
      final deep = scroll.offset;
      expect(deep, greaterThan(300));
      expect(tester.getTopLeft(find.byTooltip('帖子排序')).dy, lessThan(12));

      // Threads 式：左右切页签只换内容，不改变整体滚动位置。
      await tester.tap(profileTab(3));
      await tester.pumpAndSettle();
      expect(scroll.offset, closeTo(deep, 1));

      // 切回原页签也不会跳回旧深度或跳到顶部（位置一直没动）。
      await tester.tap(profileTab(0));
      await tester.pumpAndSettle();
      expect(scroll.offset, closeTo(deep, 1));
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );
}

/// 自己的帖子：编辑与删除均可用。
class _OwnPostApi extends PersonalRepostsApi {
  @override
  Future<PostResponse<PostCapabilities>> capabilities({
    required String sessionToken,
    required String postId,
  }) async => response(
    PostCapabilities(postId: postId, canDelete: true, canEdit: true),
  );
}

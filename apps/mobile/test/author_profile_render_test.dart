import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/posts/feed_api.dart';

import 'feed_ui_test.dart' as feed;

const _render = ValueKey('author-profile-render');

Future<void> _realFonts() async {
  for (final face in ['Regular', 'Medium', 'Bold']) {
    await (FontLoader(
      'Roboto',
    )..addFont(rootBundle.load('assets/fonts/Roboto-$face.ttf'))).load();
  }
  await (FontLoader(
    'HhNotoSansSC',
  )..addFont(rootBundle.load('assets/fonts/NotoSansSC-Variable.ttf'))).load();
  await (FontLoader(
    'MaterialIcons',
  )..addFont(rootBundle.load('fonts/MaterialIcons-Regular.otf'))).load();
}

Future<void> _capture(WidgetTester tester, String name) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(_render),
  );
  await tester.runAsync(() async {
    final image = await boundary.toImage(pixelRatio: 2);
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    final directory = Directory('build/author-profile-render');
    await directory.create(recursive: true);
    await File('${directory.path}/$name.png')
        .writeAsBytes(data!.buffer.asUint8List());
    image.dispose();
  });
}

void main() {
  testWidgets('author profile real-font render follows both themes', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(402, 874);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();

    const author = PostAuthor(
      state: PostAuthorState.active,
      avatar: 'default-v1',
      nickname: '林间来信',
    );
    final posts = [
      FeedPost(
        postId: feed.postId,
        channelId: feed.channelId,
        title: '今天的校园',
        body: '傍晚的风刚刚好，分享一段平常的校园时光。操场的灯刚亮，图书馆门口还有人抱着书慢慢走。',
        author: author,
        publishedAt: DateTime.utc(2026, 10, 19, 10, 42),
        viewCount: 1260,
        imageCount: 1,
      ),
      FeedPost(
        postId: '44444444-4444-4444-8444-444444444444',
        channelId: feed.otherChannelId,
        title: '周末想找个安静的自习搭子',
        body: '打算周六上午去图书馆，各学各的，中午可以一起吃饭。学累了就去校园里走走。',
        author: author,
        publishedAt: DateTime.utc(2026, 10, 17, 1, 16),
        viewCount: 806,
      ),
      FeedPost(
        postId: '55555555-5555-4555-8555-555555555555',
        channelId: feed.channelId,
        title: '借到的那本旧书里有一张手写便签',
        body: '没有署名，只写了“谢谢你把这一页读下去”。突然觉得图书馆里那些被反复借阅的书，也像在慢慢认识彼此。',
        author: author,
        publishedAt: DateTime.utc(2026, 10, 14, 13, 3),
        viewCount: 439,
        imageCount: 2,
      ),
    ];

    for (final brightness in Brightness.values) {
      final fixture = await feed.FeedFixture.create();
      addTearDown(fixture.dispose);
      fixture.api.authorProfileFor = (postId, cursor) =>
          AuthorProfile(author: author, items: posts, nextCursor: null);
      await tester.pumpWidget(
        RepaintBoundary(
          key: _render,
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: HhTelegramTheme.create(brightness),
            home: AuthorProfileScreen(
              controller: fixture.controller,
              postId: feed.postId,
              channelNames: const {
                feed.channelId: '吐槽',
                feed.otherChannelId: '搭子',
              },
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('作者主页'), findsOneWidget);
      expect(find.text('今天的校园'), findsOneWidget);
      expect(find.byKey(const ValueKey('author-profile-more')), findsOneWidget);
      expect(tester.takeException(), isNull);
      await _capture(tester, brightness.name);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    }
  });
}

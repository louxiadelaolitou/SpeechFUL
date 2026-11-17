import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/messaging/dm_governance_ui.dart';
import 'package:hnuhole_mobile/src/posts/author_block_api.dart';

import 'feed_ui_test.dart' as feed;

const _render = ValueKey('blacklist-render');

PostAuthor _author(String nickname) => PostAuthor(
  state: PostAuthorState.active,
  avatar: 'default-v1',
  nickname: nickname,
);

BlockedAuthor _block(String id, String nickname) =>
    BlockedAuthor(blockId: id, author: _author(nickname), blockedAt: feed.at);

Widget _screen(
  feed.FeedFixture fixture, {
  Brightness brightness = Brightness.light,
  double textScale = 1,
}) => RepaintBoundary(
  key: _render,
  child: MaterialApp(
    debugShowCheckedModeBanner: false,
    theme: HhTelegramTheme.create(brightness),
    builder: (context, child) => MediaQuery(
      data: MediaQuery.of(context)
          .copyWith(textScaler: TextScaler.linear(textScale)),
      child: child!,
    ),
    home: DmBlacklistScreen(posts: fixture.controller),
  ),
);

Future<void> _capture(WidgetTester tester, String name) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(_render),
  );
  await tester.runAsync(() async {
    final image = await boundary.toImage(pixelRatio: 2);
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    final directory = Directory('build/blacklist-render');
    await directory.create(recursive: true);
    await File('${directory.path}/$name.png')
        .writeAsBytes(data!.buffer.asUint8List());
    image.dispose();
  });
}

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

void main() {
  testWidgets('blacklist uses a stable loading state before rows arrive', (
    tester,
  ) async {
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    final wait = Completer<PostResponse<BlockedAuthorPage>>();
    f.api.blockedAuthorsWait = wait;
    await tester.pumpWidget(_screen(f));
    await tester.pump();
    expect(find.byKey(const ValueKey('blacklist-loading')), findsOneWidget);
    expect(find.text('暂无记录'), findsNothing);
    wait.complete(feed.reply(BlockedAuthorPage([_block('block-a', '林间来信')])));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('blacklist-loading')), findsNothing);
    expect(find.text('林间来信'), findsOneWidget);
  });

  testWidgets('blacklist cancel leaves the block intact', (tester) async {
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.add(_block('block-a', '林间来信'));
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('author-unblock:block-a')));
    await tester.pumpAndSettle();
    expect(find.text('解除拉黑？'), findsNothing);
    expect(
      find.byKey(const ValueKey('author-unblock-confirm')),
      findsOneWidget,
    );
    await tester.tap(find.byKey(const ValueKey('author-unblock-cancel')));
    await tester.pumpAndSettle();
    expect(f.api.calls.where((call) => call.startsWith('unblock:')), isEmpty);
    expect(find.byKey(const ValueKey('author-block:block-a')), findsOneWidget);
  });

  testWidgets('blacklist keeps a row when unblock fails and retries', (
    tester,
  ) async {
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.add(_block('block-a', '林间来信'));
    f.api.unblockAuthorFailure = const PostFailure(
      kind: PostFailureKind.transport,
    );
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('author-unblock:block-a')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('author-unblock-confirm')));
    await tester.pumpAndSettle();
    expect(find.text('暂时无法解除，请重试。'), findsOneWidget);
    expect(find.byKey(const ValueKey('author-block:block-a')), findsOneWidget);
    f.api.unblockAuthorFailure = null;
    await tester.tap(find.byKey(const ValueKey('author-unblock:block-a')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('author-unblock-confirm')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('author-block:block-a')), findsNothing);
    expect(find.text('暂无记录'), findsOneWidget);
  });

  testWidgets('blacklist drops old rows when the account session changes', (
    tester,
  ) async {
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.add(_block('block-a', '林间来信'));
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    expect(find.text('林间来信'), findsOneWidget);
    await f.auth.sessions.logout();
    await tester.pumpAndSettle();
    expect(find.text('林间来信'), findsNothing);
    expect(find.text('会话已变化，请重新进入'), findsOneWidget);
  });

  testWidgets('blacklist renders narrow large text in light and dark themes', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(320, 740);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.addAll([
      _block('block-a', '林间来信'),
      _block('block-b', '一个很长很长的昵称用来检查窄屏大字不会挤坏解除按钮'),
      _block('block-c', '晚风'),
      _block('block-d', '旧书店的猫'),
      _block('block-e', '北门散步的人'),
    ]);
    for (final brightness in Brightness.values) {
      await tester.pumpWidget(
        _screen(f, brightness: brightness, textScale: 1.8),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      expect(find.byType(BackButton), findsNothing);
      expect(
        find.byKey(const ValueKey('author-unblock:block-a')),
        findsOneWidget,
      );
      await _capture(tester, '${brightness.name}-320-large');
    }
  });

  testWidgets('blacklist renders the empty state', (tester) async {
    tester.view.physicalSize = const Size(402, 874);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    expect(find.text('暂无记录'), findsOneWidget);
    expect(find.byKey(const ValueKey('blacklist-empty')), findsOneWidget);
    await _capture(tester, 'empty-402');
  });

  testWidgets('blacklist renders the populated list', (tester) async {
    tester.view.physicalSize = const Size(402, 874);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.addAll([
      _block('block-a', '林间来信'),
      _block('block-b', '晚风'),
      _block('block-c', '旧书店的猫'),
      _block('block-d', '北门散步的人'),
    ]);
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    expect(
      find.byKey(const ValueKey('author-unblock:block-a')),
      findsOneWidget,
    );
    await _capture(tester, 'list-402');
  });

  testWidgets('blacklist renders the unblock confirmation sheet', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(402, 874);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();
    final f = await feed.FeedFixture.create();
    addTearDown(f.dispose);
    f.api.authorBlocks.add(_block('block-a', '一个很长很长的昵称用来检查确认面板换行'));
    await tester.pumpWidget(_screen(f));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('author-unblock:block-a')));
    await tester.pumpAndSettle();
    expect(
      find.byKey(const ValueKey('author-unblock-confirm')),
      findsOneWidget,
    );
    expect(find.byKey(const ValueKey('author-unblock-handle')), findsOneWidget);
    await _capture(tester, 'confirm-402');
  });
}

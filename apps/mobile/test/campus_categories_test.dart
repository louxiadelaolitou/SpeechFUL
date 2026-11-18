import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/localization/app_localizations.dart';
import 'package:hnuhole_mobile/src/posts/feed_api.dart';
import 'package:hnuhole_mobile/src/posts/x_feed_controls.dart';

const _names = [
  '校园吐槽',
  '避雷指南',
  '吐血推荐',
  '搭子寻觅',
  '情感树洞',
  '互助问答',
  '人工智能',
  '社团活动',
  '求职经验',
  '校友评论',
];
final _channels = [
  for (var i = 0; i < _names.length; i++)
    Channel(
      id: '5f6f4f88-8f64-4bb2-9d34-${(i + 1).toString().padLeft(12, '0')}',
      code: ChannelDirectory.catalogCodes[i],
      name: _names[i],
      initiallyVisible: i < 5,
      displayOrder: (i + 1) * 10,
    ),
];
final _views = [
  for (final channel in _channels)
    FeedChannelView(channel.id, channel.name, Icons.chat_bubble_outline),
];

Future<void> _capture(WidgetTester tester, String name) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(const ValueKey('capture')),
  );
  await tester.runAsync(() async {
    final image = await boundary.toImage();
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    final directory = await Directory('build/campus-categories-render')
        .create(recursive: true);
    await File('${directory.path}/$name.png')
        .writeAsBytes(bytes!.buffer.asUint8List());
    image.dispose();
  });
}

void main() {
  test(
    'complete ten-category directory preserves identities and server names',
    () {
      final result = ChannelDirectory.validate(_channels.reversed);
      expect(result.map((c) => c.name), _names);
      expect(result.map((c) => c.id), _channels.map((c) => c.id));
      expect(ChannelDirectory.validate(_channels.take(7)), hasLength(7));
      for (final count in [0, 6, 8, 9]) {
        expect(
          () => ChannelDirectory.validate(_channels.take(count)),
          throwsFormatException,
        );
      }
      expect(
        () =>
            ChannelDirectory.validate([..._channels.take(9), _channels.first]),
        throwsFormatException,
      );
      expect(
        () => ChannelDirectory.validate([
          ..._channels.take(9),
          Channel(
            id: _channels.last.id,
            code: 'unknown',
            name: '未知',
            initiallyVisible: false,
            displayOrder: 100,
          ),
        ]),
        throwsFormatException,
      );
    },
  );

  test('all ten categories can be blocked and serialized', () {
    final ids = _channels.map((c) => c.id).toList();
    expect(
      FeedPreferences.fromJson({'blockedChannelIds': ids}).blockedChannelIds,
      ids.toSet(),
    );
    expect(
      () => FeedPreferences([...ids, '00000000-0000-4000-8000-000000000011']),
      throwsFormatException,
    );
    expect(
      () => FeedPreferences.fromJson({
        'blockedChannelIds': [...ids, ids.first],
      }),
      throwsFormatException,
    );
  });

  test('every new category has all supported translations', () {
    for (final name in _names) {
      expect(uiTranslations.containsKey(name), isTrue);
      for (final language in AppLanguage.values) {
        expect(translateUi(name, language), isNotEmpty);
      }
    }
  });

  for (final config in [
    (size: const Size(375, 812), scale: 1.0, brightness: Brightness.light),
    (size: const Size(320, 640), scale: 2.0, brightness: Brightness.dark),
    (size: const Size(812, 375), scale: 1.0, brightness: Brightness.light),
  ]) {
    testWidgets(
      'ten-category picker is scrollable at ${config.size}, scale ${config.scale}',
      (tester) async {
        await tester.runAsync(() async {
          final font = FontLoader('HhNotoSansSC')
            ..addFont(rootBundle.load('assets/fonts/NotoSansSC-Variable.ttf'));
          await font.load();
          final latin = FontLoader('Roboto')
            ..addFont(rootBundle.load('assets/fonts/Roboto-Regular.ttf'));
          await latin.load();
        });
        tester.view.devicePixelRatio = 1;
        tester.view.physicalSize = config.size;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        final authority = ChangeNotifier();
        addTearDown(authority.dispose);
        String? selected;
        await tester.pumpWidget(
          RepaintBoundary(
            key: const ValueKey('capture'),
            child: MaterialApp(
              debugShowCheckedModeBanner: false,
              theme: HhTelegramTheme.create(config.brightness),
              builder: (context, child) => MediaQuery(
                data: MediaQuery.of(context).copyWith(
                  textScaler: TextScaler.linear(config.scale),
                  disableAnimations: true,
                ),
                child: child!,
              ),
              home: Builder(
                builder: (context) => Scaffold(
                  body: TextButton(
                    onPressed: () => showFeedChannelPicker(
                      context,
                      channels: _views,
                      authority: authority,
                      isCurrent: () => true,
                      selected: null,
                      includeRecommended: false,
                      onSelected: (id) => selected = id,
                    ),
                    child: const Text('打开分类'),
                  ),
                ),
              ),
            ),
          ),
        );
        await tester.tap(find.text('打开分类'));
        await tester.pumpAndSettle();
        expect(find.text('校园吐槽'), findsOneWidget);
        final list = find.byKey(const ValueKey('channel-options'));
        final scrollable = find.descendant(
          of: list,
          matching: find.byType(Scrollable),
        );
        for (final channel in _channels) {
          final option = find.byKey(ValueKey('choose-channel:${channel.id}'));
          await tester.scrollUntilVisible(option, 120, scrollable: scrollable);
          expect(find.text(channel.name), findsOneWidget);
          expect(tester.getSize(option).height, greaterThanOrEqualTo(48));
        }
        expect(tester.takeException(), isNull);
        await tester.pumpAndSettle();
        await _capture(
          tester,
          'picker-${config.size.width.toInt()}-scale-${config.scale}',
        );
        await tester.tap(find.text('校友评论'));
        await tester.pumpAndSettle();
        expect(selected, _channels.last.id);
        expect(find.byKey(const ValueKey('channel-options')), findsNothing);
      },
    );
  }
}

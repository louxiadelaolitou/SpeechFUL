import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/design_system/feedback.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';

Widget _host({
  bool navigate = false,
  Brightness brightness = Brightness.light,
}) => MaterialApp(
  theme: ThemeData(
    brightness: brightness,
    extensions: [
      brightness == Brightness.dark
          ? HhTelegramPalette.dark
          : HhTelegramPalette.light,
    ],
  ),
  home: Scaffold(
    body: Builder(
      builder: (context) => Column(
        children: [
          FilledButton(
            onPressed: () => showHhFeedback(context, '统一提示'),
            child: const Text('show'),
          ),
          if (navigate)
            FilledButton(
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => const Scaffold(body: Text('next')),
                ),
              ),
              child: const Text('next'),
            ),
        ],
      ),
    ),
  ),
);

/// 统一按手机视口跑用例：关闭阈值是胶囊宽度的比例，宽屏上 200px 拖拽不越界。
void _usePhone(WidgetTester tester) {
  tester.view.physicalSize = const Size(390, 844);
  tester.view.devicePixelRatio = 1;
  addTearDown(() {
    tester.view.resetPhysicalSize();
    tester.view.resetDevicePixelRatio();
  });
}

Future<void> _show(WidgetTester tester) async {
  await tester.tap(find.text('show'));
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 500));
}

void main() {
  testWidgets('统一反馈两秒自动关闭且带明显叉号', (tester) async {
    _usePhone(tester);
    await tester.pumpWidget(_host());
    await _show(tester);
    expect(find.byKey(const ValueKey('hh-feedback')), findsOneWidget);
    expect(find.byKey(const ValueKey('hh-feedback-close')), findsOneWidget);
    expect(find.byIcon(Icons.close_rounded), findsOneWidget);
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback')), findsNothing);
  });

  testWidgets('叉号快速关闭且连续触发只保留最新一条', (tester) async {
    _usePhone(tester);
    await tester.pumpWidget(_host());
    await _show(tester);
    await tester.tap(find.byKey(const ValueKey('hh-feedback-close')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback')), findsNothing);

    await _show(tester);
    await tester.tap(find.text('show'));
    await tester.pump();
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback')), findsOneWidget);
    expect(find.text('统一提示'), findsOneWidget);
  });

  for (final dx in [-200.0, 200.0]) {
    testWidgets('左右滑动移除统一反馈 $dx', (tester) async {
      _usePhone(tester);
      await tester.pumpWidget(_host());
      await _show(tester);
      await tester.drag(
        find.byKey(const ValueKey('hh-feedback-swipe')),
        Offset(dx, 0),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('hh-feedback')), findsNothing);
    });
  }

  testWidgets('提示胶囊紧凑且配色跟随主题（浅色与深色）', (tester) async {
    for (final brightness in Brightness.values) {
      _usePhone(tester);
      await tester.pumpWidget(_host(brightness: brightness));
      await _show(tester);
      final palette = HhTelegramPalette.of(
        tester.element(find.byKey(const ValueKey('hh-feedback-card'))),
      );
      final card = tester.widget<Material>(
        find.byKey(const ValueKey('hh-feedback-card')),
      );
      expect(card.color, palette.surface);
      expect(card.elevation, greaterThan(0));
      expect((card.shape! as RoundedRectangleBorder).side.color, palette.line);
      expect(tester.widget<Text>(find.text('统一提示')).style!.color, palette.ink);
      expect(
        tester
            .widget<IconButton>(find.byKey(const ValueKey('hh-feedback-close')))
            .iconSize,
        18,
      );
      expect(
        tester.getSize(find.byKey(const ValueKey('hh-feedback-card'))).height,
        lessThan(44),
      );
      await tester.pumpWidget(const SizedBox());
    }
  });

  testWidgets('左右滑动时胶囊、图标与文字一起位移', (tester) async {
    _usePhone(tester);
    await tester.pumpWidget(_host());
    await _show(tester);
    final card = find.byKey(const ValueKey('hh-feedback-card'));
    final close = find.byKey(const ValueKey('hh-feedback-close'));
    final text = find.text('统一提示');
    final cardStart = tester.getTopLeft(card);
    final closeStart = tester.getTopLeft(close);
    final textStart = tester.getTopLeft(text);

    final gesture = await tester.startGesture(
      tester.getCenter(find.byKey(const ValueKey('hh-feedback-swipe'))),
    );
    await gesture.moveBy(const Offset(-60, 0));
    await tester.pump();

    expect(tester.getTopLeft(card).dx, closeTo(cardStart.dx - 60, .01));
    expect(tester.getTopLeft(close).dx, closeTo(closeStart.dx - 60, .01));
    expect(tester.getTopLeft(text).dx, closeTo(textStart.dx - 60, .01));
    await gesture.up();
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback-card')), findsOneWidget);
  });

  testWidgets('提示显示期间切换页面仍可关闭', (tester) async {
    _usePhone(tester);
    await tester.pumpWidget(_host(navigate: true));
    await _show(tester);
    await tester.tap(find.text('next'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback')), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('hh-feedback-close')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('hh-feedback')), findsNothing);
  });
}

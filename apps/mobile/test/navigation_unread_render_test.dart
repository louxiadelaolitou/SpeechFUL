import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/navigation/app_navigation.dart';

const _boundary = ValueKey('navigation-unread-render');

Future<void> _realFonts() async {
  for (final face in ['Regular', 'Medium', 'Bold']) {
    await (FontLoader(
      'Roboto',
    )..addFont(rootBundle.load('assets/fonts/Roboto-$face.ttf'))).load();
  }
  await (FontLoader(
    'HhNotoSansSC',
  )..addFont(rootBundle.load('assets/fonts/NotoSansSC-Variable.ttf'))).load();
}

Future<void> _capture(WidgetTester tester, String name) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(_boundary),
  );
  await tester.runAsync(() async {
    final image = await boundary.toImage(pixelRatio: 2);
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    final directory = Directory('build/navigation-unread-render');
    await directory.create(recursive: true);
    await File('${directory.path}/$name.png')
        .writeAsBytes(data!.buffer.asUint8List());
    image.dispose();
  });
}

void main() {
  testWidgets('navigation unread dots render on light and dark surfaces', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(320, 180);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _realFonts();
    for (final brightness in Brightness.values) {
      await tester.pumpWidget(
        RepaintBoundary(
          key: _boundary,
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: HhTelegramTheme.create(brightness),
            home: Scaffold(
              body: const SizedBox.expand(),
              bottomNavigationBar: HhAppNavigation(
                selected: AppDestination.home,
                onSelected: (_) {},
                composeSlot: true,
                onCompose: () {},
                messagesUnread: true,
                directMessagesUnread: true,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.byKey(const ValueKey('navigation-unread:messages')),
        findsOneWidget,
      );
      await _capture(tester, brightness.name);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    }
  });
}

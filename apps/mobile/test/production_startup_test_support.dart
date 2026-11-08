import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/main.dart' as app;

/// Decode the bundled logo outside the test clock, then drive both startup
/// frames before inspecting the production dependency graph.
Future<void> startProductionApp(WidgetTester tester) async {
  // Host startup has no system credential provider. Resolve its capability
  // query explicitly instead of leaving a native timeout on the test clock.
  const passkeys = MethodChannel('hnuhole/auth_passkey');
  final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  messenger.setMockMethodCallHandler(passkeys, (call) async =>
      call.method == 'available' ? false : null);
  addTearDown(() => messenger.setMockMethodCallHandler(passkeys, null));
  await tester.pumpWidget(const MaterialApp(home: SizedBox.shrink()));
  await tester.runAsync(
    () => precacheImage(
      const AssetImage('assets/branding/welcome-shush-mark.png'),
      tester.element(find.byType(SizedBox).first),
    ),
  );
  final startup = app.main();
  await tester.pump();
  await tester.pump();
  await startup;
  await tester.pumpAndSettle();
}

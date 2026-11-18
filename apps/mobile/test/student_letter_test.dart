import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';
import 'package:hnuhole_mobile/src/localization/app_localizations.dart';
import 'package:hnuhole_mobile/src/localization/translations_student_letter.dart';

import 'identity_test_support.dart';
import 'login_home_test_support.dart';

const _dialogKey = ValueKey('student-letter-dialog');
const _buttonKey = ValueKey('student-letter-read');
final _scroll = find.descendant(
  of: find.byKey(const ValueKey('student-letter-scroll')),
  matching: find.byType(Scrollable),
);

Future<void> _read(WidgetTester tester) async {
  await tester.drag(
    find.byKey(const ValueKey('student-letter-scroll')),
    const Offset(0, -10000),
  );
  await tester.pumpAndSettle();
  expect(
    tester.widget<FilledButton>(find.byKey(_buttonKey)).onPressed,
    isNotNull,
  );
  expect(find.byKey(_buttonKey).hitTestable(), findsOneWidget);
  expect(
    tester
        .widget<AnimatedOpacity>(
          find.byKey(const ValueKey('student-letter-scroll-cue')),
        )
        .opacity,
    0,
  );
  expect(
    tester
        .widget<FractionallySizedBox>(
          find.descendant(
            of: find.byKey(const ValueKey('student-letter-progress')),
            matching: find.byType(FractionallySizedBox),
          ),
        )
        .heightFactor,
    closeTo(1, .02),
  );
}

Future<void> _acknowledge(WidgetTester tester) async {
  await _read(tester);
  await tester.tap(find.byKey(_buttonKey));
  await tester.pumpAndSettle();
  expect(find.byKey(_dialogKey), findsNothing);
}

Future<void> _dispose(WidgetTester tester) async {
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pump();
}

void main() {
  testWidgets('signed-out first entry requires reading to the end', (
    tester,
  ) async {
    final fixture = HomeTestFixture();
    await tester.pumpWidget(
      await fixture.create(signedIn: false, studentLetterRead: false),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(_dialogKey), findsOneWidget);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.text(studentLetterTitle), findsOneWidget);
    for (final paragraph in studentLetterParagraphs) {
      expect(find.text(paragraph), findsOneWidget);
    }
    expect(find.text(studentLetterSignature), findsOneWidget);
    expect(find.byKey(_buttonKey).hitTestable(), findsNothing);
    expect(
      tester.widget<FilledButton>(find.byKey(_buttonKey)).onPressed,
      isNull,
    );
    final semantics = tester.ensureSemantics();
    expect(find.bySemanticsLabel('我已阅读'), findsNothing);
    semantics.dispose();

    final dialog = tester.widget<Dialog>(find.byKey(_dialogKey));
    expect(dialog.backgroundColor, HhTelegramPalette.light.raised);
    final accent = tester.widget<Container>(
      find.byKey(const ValueKey('student-letter-accent')),
    );
    expect(
      (accent.decoration! as BoxDecoration).color,
      HhTelegramPalette.light.action,
    );
    expect(
      tester
          .widget<ColoredBox>(
            find.byKey(const ValueKey('student-letter-progress-fill')),
          )
          .color,
      HhTelegramPalette.light.action,
    );
    final body = tester.widget<DefaultTextStyle>(
      find.byKey(const ValueKey('student-letter-body')),
    );
    expect(body.style.fontFamily, 'Roboto');
    expect(body.style.fontFamilyFallback, contains('HhNotoSansSC'));

    await tester.tapAt(const Offset(2, 2));
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byKey(_dialogKey), findsOneWidget);
    expect(await fixture.studentLetters.hasRead(), isFalse);

    await _acknowledge(tester);
    expect(await fixture.studentLetters.hasRead(), isTrue);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.byKey(const ValueKey('auth-start')), findsOneWidget);
    await _dispose(tester);
  });

  testWidgets('signed-in entry never shows the letter', (tester) async {
    final fixture = HomeTestFixture();
    await tester.pumpWidget(
      await fixture.create(signedIn: true, studentLetterRead: false),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(_dialogKey), findsNothing);
    expect(find.byType(FeedScreen), findsOneWidget);
    expect(await fixture.studentLetters.hasRead(), isFalse);

    await openSettingsFromHome(tester);
    await tester.drag(
      find.byKey(const ValueKey('settings-list')),
      const Offset(0, -1000),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('settings-student-letter')), findsNothing);
    expect(find.text('致同学的一封信'), findsNothing);
    await _dispose(tester);
  });

  testWidgets('restoring a session stays closed until signed out is final', (
    tester,
  ) async {
    final fixture = HomeTestFixture();
    final hold = Completer<void>();
    fixture.vault.nextReadWait = hold;
    await tester.pumpWidget(
      await fixture.create(signedIn: false, studentLetterRead: false),
    );
    await tester.pump();

    expect(fixture.sessions.status, AuthStatus.restoring);
    expect(find.byKey(_dialogKey), findsNothing);

    hold.complete();
    await tester.pumpAndSettle();
    expect(fixture.sessions.status, AuthStatus.signedOut);
    expect(find.byKey(_dialogKey), findsOneWidget);
    await _dispose(tester);
  });

  testWidgets(
    'one installation acknowledges once; a new installation repeats',
    (tester) async {
      final first = HomeTestFixture();
      await tester.pumpWidget(
        await first.create(signedIn: false, studentLetterRead: false),
      );
      await tester.pumpAndSettle();
      await _acknowledge(tester);
      final receipt = first.studentLetterVault.value;
      expect(receipt, isNotNull);
      await _dispose(tester);

      final restarted = HomeTestFixture()..studentLetterVault.value = receipt;
      await tester.pumpWidget(
        await restarted.create(signedIn: false, studentLetterRead: false),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsNothing);
      expect(find.byType(AuthWelcomeScreen), findsOneWidget);
      await _dispose(tester);

      final newInstallation = HomeTestFixture();
      await tester.pumpWidget(
        await newInstallation.create(signedIn: false, studentLetterRead: false),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsOneWidget);
      await _dispose(tester);
    },
  );

  testWidgets(
    'save failure keeps the letter open and retry persists the receipt',
    (tester) async {
      final fixture = HomeTestFixture();
      await tester.pumpWidget(
        await fixture.create(signedIn: false, studentLetterRead: false),
      );
      await tester.pumpAndSettle();
      fixture.studentLetterVault.failContaining = 'student-letter';
      await _read(tester);
      await tester.tap(find.byKey(_buttonKey));
      await tester.pumpAndSettle();
      expect(find.text('阅读记录未保存，请重试。'), findsOneWidget);
      expect(find.byKey(_dialogKey), findsOneWidget);
      expect(await fixture.studentLetters.hasRead(), isFalse);

      await tester.tap(find.byKey(_buttonKey));
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsNothing);
      expect(await fixture.studentLetters.hasRead(), isTrue);
      await _dispose(tester);
    },
  );

  final render = Platform.environment['STUDENT_LETTER_RENDER_DIR'];
  for (final scenario in const [
    (Size(375, 812), 1.0, Brightness.light, AppLanguage.simplifiedChinese),
    (Size(320, 640), 2.0, Brightness.dark, AppLanguage.simplifiedChinese),
    (Size(812, 375), 1.0, Brightness.light, AppLanguage.english),
    (Size(768, 1024), 1.0, Brightness.dark, AppLanguage.traditionalChinese),
    (Size(375, 812), 1.0, Brightness.light, AppLanguage.japanese),
  ]) {
    testWidgets('responsive letter $scenario', (tester) async {
      if (render != null) {
        await tester.runAsync(() async {
          for (final entry in {
            'Roboto': ['assets/fonts/Roboto-Regular.ttf'],
            'HhNotoSansSC': ['assets/fonts/NotoSansSC-Variable.ttf'],
            'MaterialIcons': ['fonts/MaterialIcons-Regular.otf'],
          }.entries) {
            final loader = FontLoader(entry.key);
            for (final asset in entry.value) {
              loader.addFont(rootBundle.load(asset));
            }
            await loader.load();
          }
          await Directory(render).create(recursive: true);
        });
      }
      tester.view.physicalSize = scenario.$1;
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final presentation = AuthPresentationController()
        ..setLanguage(scenario.$4);
      final boundaryKey = GlobalKey();
      final store = StudentLetterStore(IdentityTestVault());
      await tester.pumpWidget(
        RepaintBoundary(
          key: boundaryKey,
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            locale: scenario.$4.locale,
            supportedLocales: appSupportedLocales,
            localizationsDelegates: appLocalizationDelegates,
            theme: HhTelegramTheme.create(scenario.$3),
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(context).copyWith(
                textScaler: TextScaler.linear(scenario.$2),
                disableAnimations: true,
                padding: const EdgeInsets.only(top: 24, bottom: 24),
              ),
              child: AuthPresentationScope(
                controller: presentation,
                child: child!,
              ),
            ),
            home: StudentLetterGate(
              store: store,
              child: const Scaffold(body: Text('entry')),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.text(translateUi(studentLetterTitle, scenario.$4)),
        findsOneWidget,
      );
      expect(find.byKey(_buttonKey).hitTestable(), findsNothing);
      expect(
        tester.state<ScrollableState>(_scroll).position.maxScrollExtent,
        greaterThan(0),
      );
      final colors = scenario.$3 == Brightness.dark
          ? HhTelegramPalette.dark
          : HhTelegramPalette.light;
      expect(
        tester
            .widget<ColoredBox>(
              find.byKey(const ValueKey('student-letter-progress-fill')),
            )
            .color,
        colors.action,
      );
      Future<void> capture(String suffix) async {
        if (render == null) return;
        final boundary =
            boundaryKey.currentContext!.findRenderObject()!
                as RenderRepaintBoundary;
        await tester.runAsync(() async {
          final bitmap = await boundary.toImage();
          final bytes = await bitmap.toByteData(format: ui.ImageByteFormat.png);
          final name =
              '${scenario.$1.width.toInt()}-${scenario.$3.name}-${scenario.$4.name}-$suffix';
          await File('$render/$name.png')
              .writeAsBytes(bytes!.buffer.asUint8List());
          bitmap.dispose();
        });
      }

      await capture('start');
      await _read(tester);
      expect(
        tester.getSize(find.byKey(_buttonKey)).height,
        greaterThanOrEqualTo(48),
      );
      expect(tester.takeException(), isNull);
      await capture('end');
      await tester.tap(find.byKey(_buttonKey));
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsNothing);
      await _dispose(tester);
      presentation.dispose();
    });
  }
}

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
  testWidgets('first home entry requires reading the letter to the end', (
    tester,
  ) async {
    final fixture = HomeTestFixture();
    await tester.pumpWidget(await fixture.create(studentLetterRead: false));
    await tester.pumpAndSettle();
    expect(find.byKey(_dialogKey), findsOneWidget);
    expect(find.text(studentLetterTitle), findsOneWidget);
    for (final paragraph in studentLetterParagraphs) {
      expect(find.text(paragraph), findsOneWidget);
    }
    expect(find.text(studentLetterSignature), findsOneWidget);
    expect(
      find.byKey(const ValueKey('student-letter-progress')),
      findsOneWidget,
    );
    expect(
      tester
          .widget<AnimatedOpacity>(
            find.byKey(const ValueKey('student-letter-scroll-cue')),
          )
          .opacity,
      1,
    );
    expect(find.byKey(_buttonKey).hitTestable(), findsNothing);
    expect(
      tester.widget<FilledButton>(find.byKey(_buttonKey)).onPressed,
      isNull,
    );
    final semantics = tester.ensureSemantics();
    expect(find.bySemanticsLabel('我已阅读'), findsNothing);
    semantics.dispose();
    await tester.tapAt(const Offset(2, 2));
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byKey(_dialogKey), findsOneWidget);
    expect(await fixture.studentLetters.hasRead(identityTestAccount), isFalse);
    expect(find.byType(FeedScreen, skipOffstage: false), findsOneWidget);
    await _acknowledge(tester);
    expect(await fixture.studentLetters.hasRead(identityTestAccount), isTrue);
    expect(find.byType(FeedScreen), findsOneWidget);
    await _dispose(tester);
  });

  testWidgets('accounts on one installation each acknowledge once', (
    tester,
  ) async {
    final fixture = HomeTestFixture();
    await tester.pumpWidget(await fixture.create(studentLetterRead: false));
    await tester.pumpAndSettle();
    await _acknowledge(tester);
    await fixture.loginB(
      tester,
      openPersonal: false,
      expectStudentLetter: true,
    );
    const b = '00000000-0000-0000-0000-000000000002';
    expect(await fixture.studentLetters.hasRead(b), isFalse);
    await _acknowledge(tester);
    await fixture.loginB(
      tester,
      account: identityTestAccount,
      name: '账号A的身份',
      openPersonal: false,
    );
    expect(find.byKey(_dialogKey), findsNothing);
    expect(await fixture.studentLetters.hasRead(b), isTrue);
    await _dispose(tester);
  });

  testWidgets(
    'relaunch remembers reading; a fresh installation presents again',
    (tester) async {
      final first = HomeTestFixture();
      await tester.pumpWidget(await first.create(studentLetterRead: false));
      await tester.pumpAndSettle();
      await _acknowledge(tester);
      await _dispose(tester);

      final restarted = HomeTestFixture()
        ..studentLetterVaults.addAll(first.studentLetterVaults);
      await tester.pumpWidget(await restarted.create(studentLetterRead: false));
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsNothing);
      expect(find.byType(FeedScreen), findsOneWidget);
      await _dispose(tester);

      final newDevice = HomeTestFixture();
      await tester.pumpWidget(await newDevice.create(studentLetterRead: false));
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsOneWidget);
      await _dispose(tester);
    },
  );

  testWidgets('interruption and authorization loss never acknowledge reading', (
    tester,
  ) async {
    final fixture = HomeTestFixture();
    await tester.pumpWidget(await fixture.create(studentLetterRead: false));
    await tester.pumpAndSettle();
    await _read(tester);
    await fixture.sessions.logout();
    await tester.pumpAndSettle();
    expect(find.byKey(_dialogKey), findsNothing);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(await fixture.studentLetters.hasRead(identityTestAccount), isFalse);
    await _dispose(tester);

    final restarted = HomeTestFixture()
      ..studentLetterVaults.addAll(fixture.studentLetterVaults);
    await tester.pumpWidget(await restarted.create(studentLetterRead: false));
    await tester.pumpAndSettle();
    expect(find.byKey(_dialogKey), findsOneWidget);
    await _dispose(tester);
  });

  testWidgets(
    'save failure keeps the letter open and retry persists the receipt',
    (tester) async {
      final fixture = HomeTestFixture();
      await tester.pumpWidget(await fixture.create(studentLetterRead: false));
      await tester.pumpAndSettle();
      fixture.studentLetterVaults[identityTestAccount]!.failContaining =
          'student-letter';
      await _read(tester);
      await tester.tap(find.byKey(_buttonKey));
      await tester.pumpAndSettle();
      expect(find.text('阅读记录未保存，请重试。'), findsOneWidget);
      expect(
        await fixture.studentLetters.hasRead(identityTestAccount),
        isFalse,
      );
      await _acknowledge(tester);
      expect(await fixture.studentLetters.hasRead(identityTestAccount), isTrue);
      await _dispose(tester);
    },
  );

  testWidgets(
    'the letter waits for initial identity setup and supports rereading',
    (tester) async {
      final fixture = HomeTestFixture();
      await tester.pumpWidget(
        await fixture.create(studentLetterRead: false, zeroIdentities: true),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsNothing);
      await tester.enterText(
        find.byKey(const ValueKey('identity-nickname')),
        '我的身份',
      );
      await tester.ensureVisible(find.byKey(const ValueKey('identity-save')));
      await tester.tap(find.byKey(const ValueKey('identity-save')));
      await tester.pumpAndSettle();
      expect(find.byKey(_dialogKey), findsOneWidget);
      await _acknowledge(tester);
      final receipt = fixture.studentLetterVaults[identityTestAccount]!.value;
      await tester.tap(find.byKey(const ValueKey('feed-account')));
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.byKey(const ValueKey('settings-student-letter')),
        100,
        scrollable: find.descendant(
          of: find.byKey(const ValueKey('settings-list')),
          matching: find.byType(Scrollable),
        ),
      );
      await tester.tap(find.byKey(const ValueKey('settings-student-letter')));
      await tester.pumpAndSettle();
      expect(find.byKey(_buttonKey).hitTestable(), findsNothing);
      await _acknowledge(tester);
      expect(fixture.studentLetterVaults[identityTestAccount]!.value, receipt);
      await _dispose(tester);
    },
  );

  testWidgets('a delayed receipt cannot release a replacement account', (
    tester,
  ) async {
    final read = Completer<bool>();
    final store = _DelayedStore(read.future);
    var readyA = 0, readyB = 0;
    Widget app(String account) => MaterialApp(
      home: StudentLetterGate(
        key: ValueKey(account),
        accountId: account,
        store: store,
        onReady: () => account == 'A' ? readyA++ : readyB++,
        child: const Scaffold(body: Text('首页')),
      ),
    );
    await tester.pumpWidget(app('A'));
    await tester.pumpAndSettle();
    await tester.pumpWidget(app('B'));
    await tester.pumpAndSettle();
    read.complete(true);
    await tester.pumpAndSettle();
    expect(readyA, 0);
    expect(readyB, 1);
    await _dispose(tester);
  });

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
            'StudentLetterSerif': ['assets/fonts/StudentLetterSerif.ttf'],
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
            home: Builder(
              builder: (context) => Scaffold(
                body: TextButton(
                  onPressed: () => unawaited(showStudentLetter(context)),
                  child: const Text('open'),
                ),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
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

class _DelayedStore extends StudentLetterStore {
  _DelayedStore(this.result) : super(vaultFor: (_) => IdentityTestVault());
  final Future<bool> result;
  @override
  Future<bool> hasRead(String accountId) => result;
}

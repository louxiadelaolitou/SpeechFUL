import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/auth/device_management_controller.dart';
import 'package:hnuhole_mobile/src/auth/account_closure_screen.dart';

import 'auth_session_controller_test.dart' as base;
import 'security_settings_test_support.dart';

class _PausedVault extends base.MemoryVault {
  Completer<void>? entered, release;
  @override
  Future<void> write(String next) async {
    final pending = release;
    if (pending != null) {
      release = null;
      entered!.complete();
      await pending.future;
    }
    await super.write(next);
  }
}

void main() {
  for (final revoke in [false, true]) {
    test(
      'metadata persistence overlapping ${revoke ? "authority suspension" : "same-token renewal"} respects the authority window',
      () async {
        final vault = _PausedVault();
        final f = await base.setup(
          vault,
          initial: AuthState(session: base.record(base.oldToken)),
        );
        await f.sessions.start();
        final c = DeviceManagementController(
          api: SecuritySettingsApi(),
          store: f.store,
          sessions: f.sessions,
        );
        addTearDown(c.dispose);
        vault.entered = Completer<void>();
        final release = Completer<void>();
        vault.release = release;
        final loading = c.load();
        await vault.entered!.future;
        f.api.renewWait = Completer<DateTime>();
        final version = f.sessions.authorityVersion;
        final renewal = revoke ? Future<void>.value() : f.sessions.onActivity();
        if (revoke) f.sessions.sessionUnavailable(base.oldToken);
        release.complete();
        await loading;
        if (revoke) {
          expect(f.sessions.authorityVersion, isNot(version));
          expect(c.device, isNull);
          expect(c.status, isNot(DeviceManagementStatus.ready));
        } else {
          f.api.renewWait!.complete(
            DateTime.now().toUtc().add(const Duration(days: 30)),
          );
          await renewal;
          expect(f.sessions.authorityVersion, version);
          expect(c.status, DeviceManagementStatus.ready);
          expect(c.device, isNotNull);
        }
      },
    );
  }
  test('same-token renewal preserves loaded device projection', () async {
    final f = await base.setup(
      base.MemoryVault(),
      initial: AuthState(session: base.record(base.oldToken)),
    );
    await f.sessions.start();
    final c = DeviceManagementController(
      api: SecuritySettingsApi(),
      store: f.store,
      sessions: f.sessions,
    );
    addTearDown(c.dispose);
    await c.load();
    expect(c.device, isNotNull);
    await f.store.update(
      (s) => s.copyWith(session: base.record(base.oldToken)),
    );
    f.api.renewWait = Completer<DateTime>();
    final version = f.sessions.authorityVersion;
    final renewal = f.sessions.onActivity();
    expect(f.sessions.authorityVersion, version);
    expect(f.sessions.isCurrentToken(base.oldToken), isTrue);
    f.api.renewWait!.complete(
      DateTime.now().toUtc().add(const Duration(days: 30)),
    );
    await renewal;
    expect(c.device, isNotNull);
  });
  testWidgets('normal renewal preserves account closure confirmation', (
    tester,
  ) async {
    final f = await base.setup(
      base.MemoryVault(),
      initial: AuthState(session: base.record(base.oldToken)),
    );
    await f.sessions.start();
    final flows = AuthFlows(
      api: f.api,
      store: f.store,
      acceptRegistrationSession: f.sessions.acceptSession,
      clearCommunityAccess: f.sessions.clearCommunityAccess,
    );
    addTearDown(flows.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: AccountClosureScreen(sessions: f.sessions, flows: flows),
      ),
    );
    final field = find.byType(TextField).first;
    await tester.enterText(field, accountClosureConfirmationPhrase);
    final confirmation = tester.widget<TextField>(field).controller!;
    expect(confirmation.text, accountClosureConfirmationPhrase);
    await f.sessions.onActivity();
    await tester.pump();
    expect(f.sessions.isCurrentToken(base.oldToken), isTrue);
    expect(confirmation.text, accountClosureConfirmationPhrase);
    expect(
      find.byKey(const ValueKey('closure-confirmation-phrase')),
      findsOneWidget,
    );
    await tester.pumpWidget(const SizedBox.shrink());
  });
}
